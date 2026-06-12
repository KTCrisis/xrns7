// xrns7 reads Renoise .xrns files headlessly: song map, exact notes as JSON,
// or play7-compatible sequences (one voice per tracker column — columns are
// monophonic by construction, the natural voice mapping).
//
//	xrns7 info song.xrns
//	xrns7 notes song.xrns --track PADS --seq 0-3
//	xrns7 play song.xrns --track CELLO --seq 0-1 | play.sh "$(cat)"
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"xrns7/internal/xrns"
)

func main() {
	if len(os.Args) < 3 {
		usage()
	}
	cmd, path := os.Args[1], os.Args[2]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	tracks := fs.String("track", "", "comma-separated track names (default: all note tracks)")
	seq := fs.String("seq", "", `sequence range "A-B" or "A" (default: whole song)`)
	fs.Parse(os.Args[3:])

	song, err := xrns.Open(path)
	if err != nil {
		fail(err)
	}
	if song.DocVersion != xrns.TestedDocVersion {
		fmt.Fprintf(os.Stderr, "xrns7: doc_version %d non testé (référence %d) — lecture best-effort\n",
			song.DocVersion, xrns.TestedDocVersion)
	}

	switch cmd {
	case "info":
		info(song)
	case "notes":
		sel, err := selection(*tracks, *seq, song)
		if err != nil {
			fail(err)
		}
		notes, err := song.Notes(sel)
		if err != nil {
			fail(err)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", " ")
		enc.Encode(notes)
	case "play":
		sel, err := selection(*tracks, *seq, song)
		if err != nil {
			fail(err)
		}
		notes, err := song.Notes(sel)
		if err != nil {
			fail(err)
		}
		out, err := playSequence(song, notes)
		if err != nil {
			fail(err)
		}
		fmt.Println(out)
	default:
		usage()
	}
}

func selection(tracks, seq string, song *xrns.Song) (xrns.Selection, error) {
	sel := xrns.Selection{From: 0, To: -1}
	if tracks != "" {
		sel.Tracks = strings.Split(tracks, ",")
	}
	if seq != "" {
		from, to, ok := strings.Cut(seq, "-")
		var err error
		if sel.From, err = strconv.Atoi(from); err != nil {
			return sel, fmt.Errorf("bad --seq %q", seq)
		}
		sel.To = sel.From
		if ok {
			if sel.To, err = strconv.Atoi(to); err != nil {
				return sel, fmt.Errorf("bad --seq %q", seq)
			}
		}
	}
	return sel, nil
}

func info(s *xrns.Song) {
	fmt.Printf("%s — %.1f BPM, %d lines/beat\n\n", orUntitled(s.Name), s.BPM, s.LPB)
	fmt.Println("tracks:")
	for i, t := range s.Tracks {
		if t.Kind == "track" {
			fmt.Printf("  %2d  %s\n", i, t.Name)
		}
	}
	if len(s.Instruments) > 0 {
		fmt.Println("\ninstruments:")
		for i, name := range s.Instruments {
			if name != "" && name != "None" {
				fmt.Printf("  %02X  %s\n", i, name)
			}
		}
	}
	fmt.Println("\nsequence:")
	for i, e := range s.Sequence {
		mark := ""
		if e.SectionStart {
			mark = "  ── " + e.Section
		}
		fmt.Printf("  %2d  pattern %2d (%d lines)%s\n", i, e.Pattern, s.Patterns[e.Pattern].Lines, mark)
	}
}

func orUntitled(s string) string {
	if s == "" || s == "Untitled" {
		return "(untitled)"
	}
	return s
}

// playSequence renders notes as a play7 sequence: one voice per (track,
// column), steps with rests filling the gaps.
func playSequence(s *xrns.Song, notes []xrns.Note) (string, error) {
	type step struct {
		Notes    []string `json:"notes,omitempty"`
		Beats    float64  `json:"beats"`
		Velocity uint8    `json:"velocity,omitempty"`
	}
	type voice struct {
		Steps []step `json:"steps"`
	}

	byVoice := map[string][]xrns.Note{}
	for _, n := range notes {
		k := fmt.Sprintf("%s/%d", n.Track, n.Column)
		byVoice[k] = append(byVoice[k], n)
	}
	keys := make([]string, 0, len(byVoice))
	for k := range byVoice {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var voices []voice
	for _, k := range keys {
		ns := byVoice[k]
		v := voice{}
		cur := 0.0
		for _, n := range ns {
			if n.Beat > cur {
				v.Steps = append(v.Steps, step{Beats: round3(n.Beat - cur)})
			}
			v.Steps = append(v.Steps, step{
				Notes: []string{n.Name}, Beats: round3(n.Beats), Velocity: n.Vel,
			})
			cur = n.Beat + n.Beats
		}
		if len(v.Steps) > 0 {
			voices = append(voices, v)
		}
	}
	if len(voices) == 0 {
		return "", fmt.Errorf("no notes in selection")
	}

	b, err := json.Marshal(map[string]any{"tempo": s.BPM, "voices": voices})
	return string(b), err
}

func round3(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }

func fail(err error) {
	fmt.Fprintln(os.Stderr, "xrns7:", err)
	os.Exit(1)
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  xrns7 info  <song.xrns>
  xrns7 notes <song.xrns> [--track A,B] [--seq 0-3]
  xrns7 play  <song.xrns> [--track A,B] [--seq 0-3]`)
	os.Exit(2)
}

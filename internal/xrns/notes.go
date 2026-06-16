package xrns

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// delayFrac maps a Renoise delay-column cell (hex "00"–"FF") to a fraction of a
// line in [0,1). Empty or unparsable = 0 (the note sits on the line). The delay
// column nudges a note within its line; ignoring it quantises onsets to lines.
func delayFrac(hex string) float64 {
	if hex == "" {
		return 0
	}
	v, err := strconv.ParseUint(strings.TrimSpace(hex), 16, 16)
	if err != nil {
		return 0
	}
	return float64(v) / 256.0
}

// Note is one extracted note event, positioned in beats.
type Note struct {
	Track      string  `json:"track"`
	Column     int     `json:"col"`
	SeqPos     int     `json:"seq"`     // position in the pattern sequence
	Pattern    int     `json:"pattern"` // pattern pool index
	Line       int     `json:"line"`    // line within the pattern
	Beat       float64 `json:"beat"`    // from the start of the extraction range
	Midi       uint8   `json:"midi"`
	Name       string  `json:"name"` // scientific pitch (C4 = 60)
	Vel        uint8   `json:"v"`
	Beats      float64 `json:"beats"`                // duration
	Instrument string  `json:"instr,omitempty"`      // hex index from the cell
	InstrName  string  `json:"instrument,omitempty"` // resolved instrument name
}

// Selection narrows extraction: track names to include (empty = all note
// tracks), names to Drop (excluded outright), a sequence range [From, To]
// inclusive (negative To = end of song), and NoDrums — drop notes a heuristic
// reads as percussion. Keep force-includes tracks the heuristic would drop
// (the escape hatch for its false positives, e.g. a pad on a kit sampler).
type Selection struct {
	Tracks  []string
	Drop    []string
	Keep    []string
	NoDrums bool
	From    int
	To      int
}

// drumTrackTokens are cryptic track-name abbreviations that denote percussion,
// matched on the whole name once trailing digits are stripped (so H2/H3 → H).
var drumTrackTokens = map[string]bool{
	"K": true, "BD": true, "KICK": true, "SN": true, "SD": true, "SNR": true,
	"H": true, "HH": true, "OH": true, "CH": true, "HAT": true, "CR": true,
	"RD": true, "CY": true, "CYM": true, "TM": true, "PRC": true, "CLP": true,
	"SHK": true, "RIM": true, "CLP1": true,
}

// drumWords are substrings that denote percussion in a descriptive track or
// instrument name ("Hi Hat", "Battery 4 Kit", "808 Snare").
var drumWords = []string{
	"kick", "snare", "hihat", "hi-hat", "hat", "crash", "ride", "tom", "perc",
	"drum", "kit", "cymbal", "clap", "snap", "rim", "tamb", "shaker", "clave",
	"conga", "bongo", "808", "909", "707", "727", "battery",
}

// fxNoteThreshold: at or below this many notes over the whole song, a track is
// likely an effect trigger (a one-shot riser/impact), not a played part.
const fxNoteThreshold = 4

func normTrackName(s string) string {
	return strings.TrimRight(strings.ToUpper(strings.TrimSpace(s)), "0123456789 ")
}

func hasDrumWord(s string) bool {
	ls := strings.ToLower(s)
	for _, w := range drumWords {
		if strings.Contains(ls, w) {
			return true
		}
	}
	return false
}

// isDrumNote reads a note as percussion from its track name (cryptic abbreviation
// or descriptive word) or its resolved instrument name — combining both signals,
// since neither alone is reliable (a kick routed through a bass synth; a pad on a
// drum-kit sampler).
func isDrumNote(n Note) bool {
	return drumTrackTokens[normTrackName(n.Track)] || hasDrumWord(n.Track) || hasDrumWord(n.InstrName)
}

func inListFold(list []string, name string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(x), name) {
			return true
		}
	}
	return false
}

// wantsTrack matches case-insensitively and ignores stray whitespace:
// `--track bass` must find "BASS", not silently select nothing.
func (sel Selection) wantsTrack(name string) bool {
	if len(sel.Tracks) == 0 {
		return true
	}
	for _, t := range sel.Tracks {
		if strings.EqualFold(strings.TrimSpace(t), name) {
			return true
		}
	}
	return false
}

// checkTracks rejects selection names that match no note track, listing what
// exists — a typo must be an error, not an empty extraction.
func (s *Song) checkTracks(sel Selection) error {
	var unknown, avail []string
	for _, t := range s.Tracks {
		if t.Kind == "track" {
			avail = append(avail, t.Name)
		}
	}
	want := append(append(append([]string{}, sel.Tracks...), sel.Drop...), sel.Keep...)
	for _, w := range want {
		found := false
		for _, name := range avail {
			if strings.EqualFold(strings.TrimSpace(w), name) {
				found = true
				break
			}
		}
		if !found {
			unknown = append(unknown, strings.TrimSpace(w))
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("no track named %s (tracks: %s)",
			strings.Join(unknown, ", "), strings.Join(avail, ", "))
	}
	return nil
}

// TrackStat summarises a note track for `info`: note count, dominant instrument,
// and a guessed class (drum / fx? / melo) — a hint, not a verdict.
type TrackStat struct {
	Name  string
	Notes int
	Instr string
	Class string
}

// TrackStats extracts the whole song once and summarises each note track, sorted
// by note count, so the player sees at a glance what to drop for a melodic
// reduction. drum wins over fx?; correct the guess with --drop / --keep.
func (s *Song) TrackStats() ([]TrackStat, error) {
	notes, err := s.Notes(Selection{From: 0, To: -1})
	if err != nil {
		return nil, err
	}
	type agg struct {
		n     int
		instr map[string]int
		drum  bool
	}
	by := map[string]*agg{}
	for _, n := range notes {
		a := by[n.Track]
		if a == nil {
			a = &agg{instr: map[string]int{}}
			by[n.Track] = a
		}
		a.n++
		a.instr[n.InstrName]++
		if isDrumNote(n) {
			a.drum = true
		}
	}
	var out []TrackStat
	for name, a := range by {
		top, topN := "", 0
		for in, c := range a.instr {
			if c > topN {
				top, topN = in, c
			}
		}
		class := "melo"
		switch {
		case a.drum:
			class = "drum"
		case a.n <= fxNoteThreshold:
			class = "fx?"
		}
		out = append(out, TrackStat{Name: name, Notes: a.n, Instr: top, Class: class})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Notes > out[j].Notes })
	return out, nil
}

// columnEvent is one cell in one tracker column, positioned at an absolute line
// from the range start (so a column's events span every pattern in the range).
type columnEvent struct {
	absLine, localLine, seqPos, patIdx int
	col                                NoteColumn
}

// colKey identifies a tracker column across the whole range: a given track's
// column index is the same physical voice in every pattern.
type colKey struct{ ti, ci int }

// Notes extracts note events over the selected sequence range, in tracker
// semantics: a note lasts until the next event in its column — another note or
// an OFF — wherever it falls. A held note rings across pattern boundaries (and
// through patterns where its column is empty) until that next event or the end
// of the range. The delay column nudges onsets within a line. Beats are
// relative to the range start.
func (s *Song) Notes(sel Selection) ([]Note, error) {
	if err := s.checkTracks(sel); err != nil {
		return nil, err
	}
	if sel.To < 0 || sel.To >= len(s.Sequence) {
		sel.To = len(s.Sequence) - 1
	}
	if sel.From < 0 || sel.From > sel.To {
		return nil, fmt.Errorf("bad sequence range %d-%d (song has %d entries)", sel.From, sel.To, len(s.Sequence))
	}

	// Build a per-column event timeline across the whole range. Absolute line
	// positions turn "gap to the next event" — even one patterns away — into a
	// plain subtraction, which is what lets a note sustain past its pattern.
	events := map[colKey][]columnEvent{}
	trackName := map[int]string{}
	lineOffset := 0 // lines elapsed since range start
	for pos := sel.From; pos <= sel.To; pos++ {
		patIdx := s.Sequence[pos].Pattern
		if patIdx < 0 || patIdx >= len(s.Patterns) {
			return nil, fmt.Errorf("sequence entry %d points to missing pattern %d", pos, patIdx)
		}
		pat := s.Patterns[patIdx]
		for ti, pt := range pat.Tracks {
			if ti >= len(s.Tracks) || s.Tracks[ti].Kind != "track" {
				continue
			}
			name := s.Tracks[ti].Name
			if !sel.wantsTrack(name) || inListFold(sel.Drop, name) {
				continue
			}
			trackName[ti] = name
			for _, l := range pt.Lines {
				// Renoise keeps cell data beyond the pattern length in the XML
				// (lines hidden by a pattern shrink) but never plays it: skip it,
				// else it would yield phantom notes and bogus durations.
				if l.Index >= pat.Lines {
					continue
				}
				for ci, c := range l.Columns {
					if c.Note == "" {
						continue
					}
					k := colKey{ti: ti, ci: ci}
					events[k] = append(events[k], columnEvent{
						absLine:   lineOffset + l.Index,
						localLine: l.Index,
						seqPos:    pos,
						patIdx:    patIdx,
						col:       c,
					})
				}
			}
		}
		lineOffset += pat.Lines
	}
	rangeLines := lineOffset // total lines: where an un-stopped note finally ends

	var out []Note
	for k, evs := range events {
		sort.Slice(evs, func(i, j int) bool { return evs[i].absLine < evs[j].absLine })
		for i, ev := range evs {
			if ev.col.Note == "OFF" {
				continue
			}
			midi, err := ParseNote(ev.col.Note)
			if err != nil {
				continue // unknown cell content; skip rather than fail the song
			}
			start := float64(ev.absLine) + delayFrac(ev.col.Delay)
			end := float64(rangeLines)
			if i+1 < len(evs) {
				end = float64(evs[i+1].absLine) + delayFrac(evs[i+1].col.Delay)
			}
			out = append(out, Note{
				Track:      trackName[k.ti],
				Column:     k.ci,
				SeqPos:     ev.seqPos,
				Pattern:    ev.patIdx,
				Line:       ev.localLine,
				Beat:       start / float64(s.LPB),
				Midi:       midi,
				Name:       SciName(midi),
				Vel:        Velocity(ev.col.Volume),
				Beats:      (end - start) / float64(s.LPB),
				Instrument: ev.col.Instrument,
				InstrName:  s.InstrumentName(ev.col.Instrument),
			})
		}
	}
	if sel.NoDrums {
		kept := out[:0]
		for _, n := range out {
			if isDrumNote(n) && !inListFold(sel.Keep, n.Track) {
				continue
			}
			kept = append(kept, n)
		}
		out = kept
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Beat != out[j].Beat {
			return out[i].Beat < out[j].Beat
		}
		if out[i].Track != out[j].Track {
			return out[i].Track < out[j].Track
		}
		return out[i].Midi < out[j].Midi
	})
	return out, nil
}

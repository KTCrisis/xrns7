package xrns

import (
	"fmt"
	"sort"
	"strings"
)

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

// Selection narrows extraction: track names (empty = all note tracks) and a
// sequence range [From, To] inclusive (negative To = end of song).
type Selection struct {
	Tracks []string
	From   int
	To     int
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
	for _, want := range sel.Tracks {
		found := false
		for _, name := range avail {
			if strings.EqualFold(strings.TrimSpace(want), name) {
				found = true
				break
			}
		}
		if !found {
			unknown = append(unknown, strings.TrimSpace(want))
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("no track named %s (tracks: %s)",
			strings.Join(unknown, ", "), strings.Join(avail, ", "))
	}
	return nil
}

// Notes extracts note events over the selected sequence range, in tracker
// semantics: a note lasts until the next event in its column — another note,
// an OFF — or the end of its pattern (v1 simplification: notes do not ring
// across pattern boundaries). Beats are relative to the range start.
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

	var out []Note
	lineOffset := 0 // lines elapsed since range start
	for pos := sel.From; pos <= sel.To; pos++ {
		patIdx := s.Sequence[pos].Pattern
		if patIdx < 0 || patIdx >= len(s.Patterns) {
			return nil, fmt.Errorf("sequence entry %d points to missing pattern %d", pos, patIdx)
		}
		pat := s.Patterns[patIdx]
		for ti, pt := range pat.Tracks {
			if ti >= len(s.Tracks) || s.Tracks[ti].Kind != "track" || !sel.wantsTrack(s.Tracks[ti].Name) {
				continue
			}
			out = append(out, extractTrack(s, pt, pat.Lines, s.Tracks[ti].Name, pos, patIdx, lineOffset)...)
		}
		lineOffset += pat.Lines
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

// columnEvent is one cell in one column, used to compute durations.
type columnEvent struct {
	line int
	col  NoteColumn
}

func extractTrack(s *Song, pt PatternTrack, patLines int, track string, seqPos, patIdx, lineOffset int) []Note {
	// Group events per column index: each tracker column is monophonic, so
	// duration = gap to the column's next event.
	perCol := map[int][]columnEvent{}
	for _, l := range pt.Lines {
		// Renoise keeps cell data beyond the pattern length in the XML (lines
		// hidden by a pattern shrink) but never plays it. Reading them would
		// yield phantom notes past the pattern end and negative durations
		// (end = patLines < line).
		if l.Index >= patLines {
			continue
		}
		for ci, c := range l.Columns {
			if c.Note == "" {
				continue
			}
			perCol[ci] = append(perCol[ci], columnEvent{line: l.Index, col: c})
		}
	}

	var out []Note
	for ci, evs := range perCol {
		sort.Slice(evs, func(i, j int) bool { return evs[i].line < evs[j].line })
		for i, ev := range evs {
			if ev.col.Note == "OFF" {
				continue
			}
			midi, err := ParseNote(ev.col.Note)
			if err != nil {
				continue // unknown cell content; skip rather than fail the song
			}
			end := patLines
			if i+1 < len(evs) {
				end = evs[i+1].line
			}
			out = append(out, Note{
				Track:      track,
				Column:     ci,
				SeqPos:     seqPos,
				Pattern:    patIdx,
				Line:       ev.line,
				Beat:       float64(lineOffset+ev.line) / float64(s.LPB),
				Midi:       midi,
				Name:       SciName(midi),
				Vel:        Velocity(ev.col.Volume),
				Beats:      float64(end-ev.line) / float64(s.LPB),
				Instrument: ev.col.Instrument,
				InstrName:  s.InstrumentName(ev.col.Instrument),
			})
		}
	}
	return out
}

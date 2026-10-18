package xrns

import "fmt"

// Span is one sequence entry placed on the timeline: where it starts and how
// long it lasts, in beats and seconds, with the section it belongs to.
type Span struct {
	Seq       int     `json:"seq"`
	Pattern   int     `json:"pattern"`
	Lines     int     `json:"lines"`
	StartBeat float64 `json:"start_beat"`
	Beats     float64 `json:"beats"`
	Start     float64 `json:"start"` // seconds
	Duration  float64 `json:"duration"`
	Section   string  `json:"section"`
	// SectionStart marks the first entry of a section inside the range, so the
	// range's first entry is one even when the section began earlier.
	SectionStart bool `json:"section_start"`
}

// Spans lays the sequence entries from..to (inclusive, to < 0 = end of song)
// on a timeline whose zero is the start of `from` — the zero of an audio
// export of that same range. An entry with no section of its own inherits the
// last one named before it, even when that name lies before the range.
func (s *Song) Spans(from, to int) ([]Span, error) {
	if to < 0 || to >= len(s.Sequence) {
		to = len(s.Sequence) - 1
	}
	if from < 0 || from > to {
		return nil, fmt.Errorf("sequence range %d-%d out of 0-%d", from, to, len(s.Sequence)-1)
	}
	if s.BPM <= 0 || s.LPB <= 0 {
		return nil, fmt.Errorf("tempo %.1f BPM / %d LPB unusable", s.BPM, s.LPB)
	}
	section := ""
	for _, e := range s.Sequence[:from] {
		if e.SectionStart {
			section = e.Section
		}
	}
	secPerBeat := 60 / s.BPM
	spans := make([]Span, 0, to-from+1)
	beat := 0.0
	for i := from; i <= to; i++ {
		e := s.Sequence[i]
		start := i == from
		if e.SectionStart {
			section, start = e.Section, true
		}
		lines := s.Patterns[e.Pattern].Lines
		beats := float64(lines) / float64(s.LPB)
		spans = append(spans, Span{
			Seq: i, Pattern: e.Pattern, Lines: lines,
			StartBeat: beat, Beats: beats,
			Start: beat * secPerBeat, Duration: beats * secPerBeat,
			Section: section, SectionStart: start,
		})
		beat += beats
	}
	return spans, nil
}

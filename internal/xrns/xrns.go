// Package xrns reads Renoise .xrns song files headlessly — no Renoise, no GUI.
// An .xrns is a zip archive holding Song.xml; this package parses the parts
// that carry symbolic music (tempo, tracks, patterns, notes, sequence) and
// ignores the rest (devices, automation, samples).
package xrns

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// TestedDocVersion is the Renoise schema version this package is tested
// against (Renoise 3.x). Other versions parse on a best-effort basis.
const TestedDocVersion = 66

// Song is the symbolic content of an .xrns file.
type Song struct {
	Name        string
	DocVersion  int // Renoise schema version (doc_version attribute)
	BPM         float64
	LPB         int      // lines per beat: the lines→beats conversion key
	Instruments []string // names, indexed by the hex instrument number in note cells
	Tracks      []Track
	Patterns    []Pattern
	Sequence    []SeqEntry
}

// Track is one sequencer track, in file order. Pattern tracks align with this
// order index-for-index (groups, master and sends included).
type Track struct {
	Name string
	Kind string // "track" | "group" | "master" | "send"
}

// Pattern is one pattern from the pool.
type Pattern struct {
	Lines  int // pattern length in lines
	Tracks []PatternTrack
}

// PatternTrack holds the non-empty lines of one track in one pattern.
type PatternTrack struct {
	Lines []Line
}

// Line is one sparse pattern line (only lines with content are stored).
type Line struct {
	Index   int
	Columns []NoteColumn
}

// NoteColumn is one note-column cell. Note is Renoise spelling ("C-4", "C#4",
// "OFF"); Volume and Delay are raw hex strings ("" = unset).
type NoteColumn struct {
	Note       string
	Instrument string
	Volume     string
	Delay      string
}

// SeqEntry is one step of the pattern sequence, with optional section marker.
type SeqEntry struct {
	Pattern      int
	SectionStart bool
	Section      string
}

// raw XML mapping — kept separate from the public types so the schema's
// shape doesn't leak into the API.
type rawSong struct {
	DocVersion     int `xml:"doc_version,attr"`
	GlobalSongData struct {
		SongName     string  `xml:"SongName"`
		BeatsPerMin  float64 `xml:"BeatsPerMin"`
		LinesPerBeat int     `xml:"LinesPerBeat"`
	} `xml:"GlobalSongData"`
	Instruments struct {
		Names []string `xml:"Instrument>Name"`
	} `xml:"Instruments"`
	Tracks struct {
		Items []rawTrack `xml:",any"`
	} `xml:"Tracks"`
	PatternPool struct {
		Patterns []rawPattern `xml:"Patterns>Pattern"`
	} `xml:"PatternPool"`
	PatternSequence struct {
		Entries []rawSeqEntry `xml:"SequenceEntries>SequenceEntry"`
	} `xml:"PatternSequence"`
}

type rawTrack struct {
	XMLName xml.Name
	Name    string `xml:"Name"`
}

type rawPattern struct {
	NumberOfLines int `xml:"NumberOfLines"`
	Tracks        struct {
		Items []rawPatternTrack `xml:",any"`
	} `xml:"Tracks"`
}

type rawPatternTrack struct {
	XMLName xml.Name
	Lines   []rawLine `xml:"Lines>Line"`
}

type rawLine struct {
	Index   int             `xml:"index,attr"`
	Columns []rawNoteColumn `xml:"NoteColumns>NoteColumn"`
}

type rawNoteColumn struct {
	Note       string `xml:"Note"`
	Instrument string `xml:"Instrument"`
	Volume     string `xml:"Volume"`
	Delay      string `xml:"Delay"`
}

type rawSeqEntry struct {
	Pattern        int    `xml:"Pattern"`
	IsSectionStart bool   `xml:"IsSectionStart"`
	SectionName    string `xml:"SectionName"`
}

var trackKinds = map[string]string{
	"SequencerTrack":       "track",
	"SequencerGroupTrack":  "group",
	"SequencerMasterTrack": "master",
	"SequencerSendTrack":   "send",
}

// Open reads an .xrns file from disk.
func Open(path string) (*Song, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == "Song.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return Parse(rc)
		}
	}
	return nil, fmt.Errorf("%s: no Song.xml inside (not an .xrns?)", path)
}

// Parse reads a Song.xml stream.
func Parse(r io.Reader) (*Song, error) {
	var raw rawSong
	if err := xml.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse Song.xml: %w", err)
	}
	if raw.GlobalSongData.LinesPerBeat <= 0 {
		return nil, fmt.Errorf("bad LinesPerBeat %d", raw.GlobalSongData.LinesPerBeat)
	}
	if raw.GlobalSongData.BeatsPerMin <= 0 {
		return nil, fmt.Errorf("bad BeatsPerMin %v", raw.GlobalSongData.BeatsPerMin)
	}

	s := &Song{
		Name:        raw.GlobalSongData.SongName,
		DocVersion:  raw.DocVersion,
		BPM:         raw.GlobalSongData.BeatsPerMin,
		LPB:         raw.GlobalSongData.LinesPerBeat,
		Instruments: raw.Instruments.Names,
	}
	for _, t := range raw.Tracks.Items {
		kind, ok := trackKinds[t.XMLName.Local]
		if !ok {
			kind = "track"
		}
		s.Tracks = append(s.Tracks, Track{Name: t.Name, Kind: kind})
	}
	for _, p := range raw.PatternPool.Patterns {
		pat := Pattern{Lines: p.NumberOfLines}
		for _, pt := range p.Tracks.Items {
			ptr := PatternTrack{}
			for _, l := range pt.Lines {
				line := Line{Index: l.Index}
				for _, c := range l.Columns {
					line.Columns = append(line.Columns, NoteColumn(c))
				}
				ptr.Lines = append(ptr.Lines, line)
			}
			pat.Tracks = append(pat.Tracks, ptr)
		}
		s.Patterns = append(s.Patterns, pat)
	}
	for _, e := range raw.PatternSequence.Entries {
		s.Sequence = append(s.Sequence, SeqEntry{
			Pattern: e.Pattern, SectionStart: e.IsSectionStart, Section: e.SectionName,
		})
	}
	return s, nil
}

var pitchClass = map[string]int{
	"C": 0, "C#": 1, "D": 2, "D#": 3, "E": 4, "F": 5,
	"F#": 6, "G": 7, "G#": 8, "A": 9, "A#": 10, "B": 11,
}

// ParseNote converts Renoise note spelling to a MIDI number. Renoise writes
// "C-4" / "C#4" with its own octave reference: Renoise C-4 = MIDI 48
// (scientific C3) — midi = octave*12 + pitch class. "OFF" and empty are not
// notes; callers handle them.
func ParseNote(s string) (uint8, error) {
	if len(s) != 3 {
		return 0, fmt.Errorf("bad note %q", s)
	}
	name := strings.TrimSuffix(s[:2], "-")
	pc, ok := pitchClass[name]
	if !ok {
		return 0, fmt.Errorf("bad note %q", s)
	}
	oct, err := strconv.Atoi(s[2:])
	if err != nil {
		return 0, fmt.Errorf("bad note %q", s)
	}
	n := oct*12 + pc
	if n < 0 || n > 127 {
		return 0, fmt.Errorf("note %q out of MIDI range", s)
	}
	return uint8(n), nil
}

// SciName renders a MIDI number in scientific pitch (C4 = 60), the convention
// keys7/play7 use. Renoise C-4 (midi 48) renders as "C3".
func SciName(midi uint8) string {
	names := [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	return fmt.Sprintf("%s%d", names[midi%12], int(midi)/12-1)
}

// Velocity converts a Renoise volume hex string (00–80, 80 = full) to MIDI
// velocity 1–127. Unset means full.
func Velocity(volHex string) uint8 {
	if volHex == "" {
		return 127
	}
	v, err := strconv.ParseUint(volHex, 16, 16)
	if err != nil || v >= 0x80 {
		return 127
	}
	if v == 0 {
		return 1
	}
	return uint8(v * 127 / 128)
}

// InstrumentName resolves a note cell's hex instrument index ("0B") to the
// instrument's name. Unknown or unset indexes return the raw string.
func (s *Song) InstrumentName(hexIdx string) string {
	i, err := strconv.ParseUint(hexIdx, 16, 16)
	if err != nil || int(i) >= len(s.Instruments) {
		return hexIdx
	}
	if n := s.Instruments[i]; n != "" && n != "None" {
		return n
	}
	return hexIdx
}

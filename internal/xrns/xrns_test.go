package xrns

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureXML = `<?xml version="1.0" encoding="UTF-8"?>
<RenoiseSong doc_version="66">
  <GlobalSongData>
    <SongName>fixture</SongName>
    <BeatsPerMin>120</BeatsPerMin>
    <LinesPerBeat>4</LinesPerBeat>
  </GlobalSongData>
  <Tracks>
    <SequencerTrack><Name>BASS</Name></SequencerTrack>
    <SequencerTrack><Name>LEAD</Name></SequencerTrack>
    <SequencerMasterTrack><Name>Master</Name></SequencerMasterTrack>
  </Tracks>
  <PatternPool>
    <Patterns>
      <Pattern>
        <NumberOfLines>16</NumberOfLines>
        <Tracks>
          <PatternTrack>
            <Lines>
              <Line index="0"><NoteColumns><NoteColumn><Note>C-3</Note><Instrument>02</Instrument><Volume>40</Volume></NoteColumn></NoteColumns></Line>
              <Line index="8"><NoteColumns><NoteColumn><Note>OFF</Note></NoteColumn></NoteColumns></Line>
            </Lines>
          </PatternTrack>
          <PatternTrack>
            <Lines>
              <Line index="0"><NoteColumns><NoteColumn><Note>E-4</Note></NoteColumn></NoteColumns></Line>
              <Line index="4"><NoteColumns><NoteColumn><Note>G-4</Note><Volume>20</Volume></NoteColumn></NoteColumns></Line>
            </Lines>
          </PatternTrack>
          <PatternMasterTrack></PatternMasterTrack>
        </Tracks>
      </Pattern>
      <Pattern>
        <NumberOfLines>8</NumberOfLines>
        <Tracks>
          <PatternTrack>
            <Lines>
              <Line index="0"><NoteColumns><NoteColumn><Note>D-3</Note></NoteColumn></NoteColumns></Line>
            </Lines>
          </PatternTrack>
          <PatternTrack></PatternTrack>
          <PatternMasterTrack></PatternMasterTrack>
        </Tracks>
      </Pattern>
    </Patterns>
  </PatternPool>
  <PatternSequence>
    <SequenceEntries>
      <SequenceEntry><Pattern>0</Pattern><IsSectionStart>true</IsSectionStart><SectionName>START</SectionName></SequenceEntry>
      <SequenceEntry><Pattern>1</Pattern></SequenceEntry>
    </SequenceEntries>
  </PatternSequence>
</RenoiseSong>`

func writeFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.xrns")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("Song.xml")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(fixtureXML))
	zw.Close()
	f.Close()
	return path
}

func TestParseNote(t *testing.T) {
	good := map[string]uint8{"C-4": 48, "C#4": 49, "A-0": 9, "B-9": 119, "G#7": 92}
	for s, want := range good {
		got, err := ParseNote(s)
		if err != nil {
			t.Errorf("ParseNote(%q): %v", s, err)
		} else if got != want {
			t.Errorf("ParseNote(%q) = %d, want %d", s, got, want)
		}
	}
	for _, s := range []string{"", "OFF", "H-4", "C-X", "C4", "C-42"} {
		if n, err := ParseNote(s); err == nil {
			t.Errorf("ParseNote(%q) = %d, want error", s, n)
		}
	}
}

func TestSciNameMapping(t *testing.T) {
	// Renoise C-4 (midi 48) is scientific C3 — the keys7/play7 convention.
	if got := SciName(48); got != "C3" {
		t.Errorf("SciName(48) = %q, want C3", got)
	}
	if got := SciName(60); got != "C4" {
		t.Errorf("SciName(60) = %q, want C4", got)
	}
}

func TestVelocity(t *testing.T) {
	cases := map[string]uint8{"": 127, "80": 127, "40": 63, "20": 31, "00": 1, "zz": 127}
	for in, want := range cases {
		if got := Velocity(in); got != want {
			t.Errorf("Velocity(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestOpenAndNotes(t *testing.T) {
	song, err := Open(writeFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if song.BPM != 120 || song.LPB != 4 {
		t.Fatalf("BPM/LPB = %v/%v", song.BPM, song.LPB)
	}
	if len(song.Tracks) != 3 || song.Tracks[2].Kind != "master" {
		t.Fatalf("tracks = %+v", song.Tracks)
	}
	if len(song.Sequence) != 2 || song.Sequence[0].Section != "START" {
		t.Fatalf("sequence = %+v", song.Sequence)
	}

	notes, err := song.Notes(Selection{From: 0, To: -1})
	if err != nil {
		t.Fatal(err)
	}
	// C-3 (cut by OFF at line 8), E-4 (cut by G-4), G-4 (to pattern end),
	// D-3 in pattern 1 (offset 16 lines = beat 4).
	type expect struct {
		name  string
		beat  float64
		beats float64
		vel   uint8
		track string
	}
	want := []expect{
		{"C2", 0, 2, 63, "BASS"},
		{"E3", 0, 1, 127, "LEAD"},
		{"G3", 1, 3, 31, "LEAD"},
		{"D2", 4, 2, 127, "BASS"},
	}
	if len(notes) != len(want) {
		t.Fatalf("got %d notes: %+v", len(notes), notes)
	}
	for i, w := range want {
		n := notes[i]
		if n.Name != w.name || n.Beat != w.beat || n.Beats != w.beats || n.Vel != w.vel || n.Track != w.track {
			t.Errorf("note %d = %+v, want %+v", i, n, w)
		}
	}
}

func TestSelection(t *testing.T) {
	song, _ := Open(writeFixture(t))

	lead, err := song.Notes(Selection{Tracks: []string{"LEAD"}, From: 0, To: -1})
	if err != nil || len(lead) != 2 {
		t.Fatalf("LEAD notes = %d (%v)", len(lead), err)
	}
	p1, err := song.Notes(Selection{From: 1, To: 1})
	if err != nil || len(p1) != 1 || p1[0].Name != "D2" || p1[0].Beat != 0 {
		t.Fatalf("seq 1 notes = %+v (%v)", p1, err)
	}
	if _, err := song.Notes(Selection{From: 5, To: 9}); err == nil {
		t.Error("out-of-range selection accepted")
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse(strings.NewReader("not xml")); err == nil {
		t.Error("accepted non-XML")
	}
	if _, err := Parse(strings.NewReader(`<RenoiseSong><GlobalSongData><LinesPerBeat>0</LinesPerBeat></GlobalSongData></RenoiseSong>`)); err == nil {
		t.Error("accepted LPB 0")
	}
}

func TestInstrumentNames(t *testing.T) {
	song, err := Parse(strings.NewReader(`<RenoiseSong>
		<GlobalSongData><BeatsPerMin>120</BeatsPerMin><LinesPerBeat>4</LinesPerBeat></GlobalSongData>
		<Instruments>
			<Instrument><Name>VST: Serum (Pad)</Name></Instrument>
			<Instrument><Name>None</Name></Instrument>
		</Instruments>
	</RenoiseSong>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := song.InstrumentName("00"); got != "VST: Serum (Pad)" {
		t.Errorf("InstrumentName(00) = %q", got)
	}
	if got := song.InstrumentName("01"); got != "01" {
		t.Errorf("InstrumentName(01) = %q, want raw index for None", got)
	}
	if got := song.InstrumentName("ZZ"); got != "ZZ" {
		t.Errorf("InstrumentName(ZZ) = %q", got)
	}
}

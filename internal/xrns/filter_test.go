package xrns

import "testing"

func TestIsDrumNote(t *testing.T) {
	cases := []struct {
		track, instr string
		want         bool
	}{
		{"K", "VST: BassLine (Deep Elektro Bass)", true},    // kick: drum track name, melodic instrument
		{"SN", "VST: BassLine (Deep Elektro Bass)", true},   // snare: same
		{"H2", "Kontakt 5 (AR80s Black Kit Lite)", true},    // hat: both signals
		{"H3", "_HAT_CL5", true},                            // hat by instrument
		{"TAMB", "CL_HAT_2", true},                          // tamb word + hat instr
		{"CR", "Kontakt 5 (AR80s Black Kit Lite)", true},    // crash abbrev + kit
		{"PAD2", "Kontakt 5 (AR80s Black Kit Lite)", true},  // documented false positive (kit sampler)
		{"BASS NIGHT", "VST: TAL-U-No-LX-V2-64", false},     // melodic
		{"BELLS", "VST: Massive (Rezforth)", false},
		{"MELO2", "VST: TAL-U-No-LX-V2-64", false},
		{"RIFF HI", "VST: TAL-U-No-LX-V2-64", false},
	}
	for _, c := range cases {
		if got := isDrumNote(Note{Track: c.track, InstrName: c.instr}); got != c.want {
			t.Errorf("isDrumNote(track=%q instr=%q) = %v, want %v", c.track, c.instr, got, c.want)
		}
	}
}

func TestNormTrackName(t *testing.T) {
	for in, want := range map[string]string{"H2": "H", "PAD2": "PAD", " sn ": "SN", "RIFF HI": "RIFF HI"} {
		if got := normTrackName(in); got != want {
			t.Errorf("normTrackName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInListFold(t *testing.T) {
	l := []string{"K", " SN ", "PAD2"}
	for _, n := range []string{"k", "sn", "PAD2"} {
		if !inListFold(l, n) {
			t.Errorf("inListFold should match %q", n)
		}
	}
	if inListFold(l, "BASS") {
		t.Error("inListFold matched BASS unexpectedly")
	}
}

package xrns

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestWriteSMF(t *testing.T) {
	notes := []Note{
		{Track: "LEAD", Beat: 0, Beats: 1, Midi: 60, Vel: 100},
		{Track: "LEAD", Beat: 1, Beats: 0.5, Midi: 64, Vel: 90},
		{Track: "BASS", Beat: 0, Beats: 2, Midi: 36, Vel: 110},
	}
	var buf bytes.Buffer
	if err := WriteSMF(&buf, notes, 120); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()

	if string(b[:4]) != "MThd" {
		t.Fatalf("header magic = %q", b[:4])
	}
	format := binary.BigEndian.Uint16(b[8:10])
	ntrks := binary.BigEndian.Uint16(b[10:12])
	div := binary.BigEndian.Uint16(b[12:14])
	// format 1, ntrks = conductor + LEAD + BASS = 3, division = smfPPQ
	if format != 1 || ntrks != 3 || div != smfPPQ {
		t.Fatalf("header format=%d ntrks=%d div=%d", format, ntrks, div)
	}
	// 120 BPM = 500000 µs/beat = FF 51 03 07 A1 20
	if !bytes.Contains(b, []byte{0xFF, 0x51, 0x03, 0x07, 0xA1, 0x20}) {
		t.Error("tempo meta for 120 BPM missing")
	}
	if n := bytes.Count(b, []byte("MTrk")); n != 3 {
		t.Errorf("MTrk chunks = %d, want 3", n)
	}
	// a C5 note-on (0x90|ch, 60, vel) must appear; channel of the first track is 0
	if !bytes.Contains(b, []byte{0x90, 60, 100}) {
		t.Error("expected note-on 60 vel 100 on channel 0")
	}

	if err := WriteSMF(&buf, nil, 120); err == nil {
		t.Error("empty note list accepted")
	}
}

func TestVarLen(t *testing.T) {
	// canonical SMF VLQ examples (MIDI spec)
	cases := map[uint32][]byte{
		0:         {0x00},
		0x40:      {0x40},
		0x7F:      {0x7F},
		0x80:      {0x81, 0x00},
		0x2000:    {0xC0, 0x00},
		0x3FFF:    {0xFF, 0x7F},
		0x100000:  {0xC0, 0x80, 0x00},
		0xFFFFFFF: {0xFF, 0xFF, 0xFF, 0x7F},
	}
	for v, want := range cases {
		if got := appendVarLen(nil, v); !bytes.Equal(got, want) {
			t.Errorf("appendVarLen(%#x) = % x, want % x", v, got, want)
		}
	}
}

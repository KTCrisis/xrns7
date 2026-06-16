package xrns

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
)

// smfPPQ is the SMF time division: ticks per quarter-note. xrns7 positions are
// in beats (quarter-notes), so one beat = smfPPQ ticks.
const smfPPQ = 480

// melodicChannels are the 16 MIDI channels minus 9 (the GM percussion channel):
// Marc's drum tracks use custom kits, not GM mappings, so routing them to
// channel 10 would make an importer play the wrong percussion. Tracks cycle
// through these.
var melodicChannels = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 10, 11, 12, 13, 14, 15}

// WriteSMF writes the notes as a Standard MIDI File (format 1): a conductor
// track carrying the tempo, then one MTrk per xrns track (first-appearance
// order). Beats map to ticks at smfPPQ; a held note's duration is honoured and
// clamped to at least one tick. Velocities are the 1..127 the extractor
// produced. This is the headless Renoise → notation bridge (feeds MuseScore).
func WriteSMF(w io.Writer, notes []Note, bpm float64) error {
	if len(notes) == 0 {
		return fmt.Errorf("no notes to write")
	}
	bw := bufio.NewWriter(w)

	// Group notes by track, preserving first-appearance order so the SMF track
	// order follows the song rather than map iteration.
	var order []string
	seen := map[string]bool{}
	byTrack := map[string][]Note{}
	for _, n := range notes {
		if !seen[n.Track] {
			seen[n.Track] = true
			order = append(order, n.Track)
		}
		byTrack[n.Track] = append(byTrack[n.Track], n)
	}

	// Header: format 1, ntrks = conductor + one per track, division = PPQ.
	bw.WriteString("MThd")
	binary.Write(bw, binary.BigEndian, uint32(6))
	binary.Write(bw, binary.BigEndian, uint16(1))
	binary.Write(bw, binary.BigEndian, uint16(1+len(order)))
	binary.Write(bw, binary.BigEndian, uint16(smfPPQ))

	if bpm <= 0 {
		bpm = 120
	}
	usPerBeat := uint32(60000000.0/bpm + 0.5)
	var cond []byte
	cond = appendVarLen(cond, 0)
	cond = append(cond, 0xFF, 0x51, 0x03, byte(usPerBeat>>16), byte(usPerBeat>>8), byte(usPerBeat))
	cond = appendEndOfTrack(cond)
	writeChunk(bw, cond)

	for i, name := range order {
		ch := melodicChannels[i%len(melodicChannels)]
		writeChunk(bw, buildTrack(byTrack[name], name, ch))
	}
	return bw.Flush()
}

// buildTrack renders one xrns track's notes as an MTrk body (without the chunk
// header): a name meta event, then time-ordered note on/off pairs.
func buildTrack(notes []Note, name string, ch byte) []byte {
	type event struct {
		tick      int
		on        bool
		midi, vel byte
	}
	var evs []event
	for _, n := range notes {
		start := int(n.Beat*smfPPQ + 0.5)
		dur := int(n.Beats*smfPPQ + 0.5)
		if dur < 1 {
			dur = 1
		}
		evs = append(evs, event{tick: start, on: true, midi: n.Midi, vel: clampVel(n.Vel)})
		evs = append(evs, event{tick: start + dur, on: false, midi: n.Midi})
	}
	// Sort by tick; at an equal tick put note-offs before note-ons so a repeated
	// pitch isn't silenced by its own re-trigger's release.
	sort.SliceStable(evs, func(i, j int) bool {
		if evs[i].tick != evs[j].tick {
			return evs[i].tick < evs[j].tick
		}
		return !evs[i].on && evs[j].on
	})

	var trk []byte
	trk = appendVarLen(trk, 0) // track-name meta at tick 0
	trk = append(trk, 0xFF, 0x03)
	trk = appendVarLen(trk, uint32(len(name)))
	trk = append(trk, []byte(name)...)

	prev := 0
	for _, e := range evs {
		trk = appendVarLen(trk, uint32(e.tick-prev))
		prev = e.tick
		if e.on {
			trk = append(trk, 0x90|ch, e.midi, e.vel)
		} else {
			trk = append(trk, 0x80|ch, e.midi, 0)
		}
	}
	return appendEndOfTrack(trk)
}

// appendVarLen appends v as a MIDI variable-length quantity (big-endian, 7 bits
// per byte, high bit set on all but the last).
func appendVarLen(b []byte, v uint32) []byte {
	var buf [5]byte
	i := len(buf) - 1
	buf[i] = byte(v & 0x7F)
	for v >>= 7; v > 0; v >>= 7 {
		i--
		buf[i] = byte(v&0x7F) | 0x80
	}
	return append(b, buf[i:]...)
}

func appendEndOfTrack(b []byte) []byte {
	return append(b, 0x00, 0xFF, 0x2F, 0x00)
}

// writeChunk wraps an MTrk body in its chunk header (type + big-endian length).
func writeChunk(w io.Writer, body []byte) {
	io.WriteString(w, "MTrk")
	binary.Write(w, binary.BigEndian, uint32(len(body)))
	w.Write(body)
}

func clampVel(v uint8) byte {
	switch {
	case v < 1:
		return 1
	case v > 127:
		return 127
	default:
		return v
	}
}

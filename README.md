# xrns7

Headless [Renoise](https://www.renoise.com) `.xrns` reader — exact symbolic
notes from song files, no Renoise running, no GUI.

An `.xrns` is a zip holding `Song.xml`. xrns7 parses the musical content —
tempo, tracks, patterns, notes, sequence with section names — and ignores the
rest (devices, automation, samples). One file read, the whole song in memory:
built for analysis pipelines and machine reading, where the in-Renoise
[MidiConvert](https://www.renoise.com/tools/midi-convert-w-extended-export)
tool (GUI-bound) doesn't fit.

xrns7 is the Renoise *read* side of the **keys7** machine ↔ music system (it
feeds play7 sequences; ReMCP is the *write* side). See
[ECOSYSTEM.md](https://github.com/KTCrisis/keys7/blob/main/ECOSYSTEM.md).

```bash
xrns7 info  song.xrns                          # song map: tracks (notes/class/instrument), sequence
xrns7 notes song.xrns --track PADS --seq 0-3   # exact notes as JSON, positions in beats
xrns7 play  song.xrns --track CELLO --seq 2-3  # play7-compatible sequence (see keys7)
xrns7 play  song.xrns --no-drums --keep PAD2   # melodic piano reduction (drop percussion)
xrns7 midi  song.xrns --track CELLO,PADS -o out.mid  # Standard MIDI File (notation / DAW import)
```

## Melodic reduction

`info` classes each track (note count, dominant instrument, a drum/fx/melo
guess) so you can see what to drop. `--no-drums` drops percussion for a piano
reduction, reading it from both the track name (cryptic abbreviations like
`K`, `SN`, `H`) and the instrument name (`Battery`, `Kit`, `HAT`) — neither
alone is reliable (a kick routed through a bass synth; a pad on a kit sampler).
The guess is a hint: `--drop A,B` excludes tracks outright, `--keep A,B`
force-keeps ones the heuristic over-eagerly tagged as drums.

## Output

`notes` emits one JSON object per note: track, column, sequence position,
pattern, line, beat position, MIDI number, scientific name, velocity
(Renoise hex volume mapped to 1–127), duration in beats, instrument.

`play` emits a sequence for [keys7](https://github.com/KTCrisis/keys7)'s play7
(`{"tempo", "voices": [{"steps": …}]}`) — **one voice per tracker column**:
columns are monophonic by construction, the natural voice mapping. Pipe it to
a MIDI instrument:

```bash
play.sh "$(xrns7 play song.xrns --track CELLO --seq 2-3)"
```

## MIDI export

`midi` writes a Standard MIDI File (format 1): a conductor track with the song
tempo, then one track per xrns track, notes positioned in ticks at 480 PPQ with
their held durations and 1–127 velocities. Same `--track` / `--seq` /
`--no-drums` selection as `notes` and `play`. Default output is `<song>.mid`;
`-o path` overrides, `-o -` writes to stdout. Drum tracks keep melodic channels
(channel 10 / GM percussion is skipped, since Renoise kits aren't GM-mapped).

```bash
xrns7 midi song.xrns --no-drums -o reduction.mid   # → import into MuseScore
```

This is the headless Renoise → notation bridge: no GUI, no MidiConvert.

## Semantics & limits

- Tracker durations: a note lasts until the next event in its column —
  another note or an `OFF` — wherever it falls. A held note rings across
  pattern boundaries (and through patterns where its column is empty) until
  that next event or the end of the selected range.
- The note delay column is applied: a cell's delay (hex `00`–`FF`) nudges the
  onset within its line by that fraction, and shortens the previous note to
  match. Onsets are no longer quantized to lines.
- Tempo and lines-per-beat are read once from the song globals; mid-song
  tempo/LPB changes (`ZTxx`/`ZLxx` master effects) are not tracked.
- Pitch mapping: Renoise `C-4` = MIDI 48 = scientific `C3` (the convention
  keys7/play7 use, A4 = 69 = 440 Hz).
- Effect columns, automation, sample data: out of scope. This reads notes.

Tested against Renoise 3.x files (`doc_version` 66) plus a synthetic fixture.

# xrns7

Headless [Renoise](https://www.renoise.com) `.xrns` reader — exact symbolic
notes from song files, no Renoise running, no GUI.

An `.xrns` is a zip holding `Song.xml`. xrns7 parses the musical content —
tempo, tracks, patterns, notes, sequence with section names — and ignores the
rest (devices, automation, samples). One file read, the whole song in memory:
built for analysis pipelines and machine reading, where the in-Renoise
[MidiConvert](https://www.renoise.com/tools/midi-convert-w-extended-export)
tool (GUI-bound) doesn't fit.

```bash
xrns7 info  song.xrns                          # song map: tracks (notes/class/instrument), sequence
xrns7 notes song.xrns --track PADS --seq 0-3   # exact notes as JSON, positions in beats
xrns7 play  song.xrns --track CELLO --seq 2-3  # play7-compatible sequence (see keys7)
xrns7 play  song.xrns --no-drums --keep PAD2   # melodic piano reduction (drop percussion)
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

## Semantics & limits (v1)

- Tracker durations: a note lasts until the next event in its column —
  another note, an `OFF` — or the end of its pattern. Notes do not ring
  across pattern boundaries (simplification).
- The note delay column is ignored: positions quantize to lines.
- Pitch mapping: Renoise `C-4` = MIDI 48 = scientific `C3` (the convention
  keys7/play7 use, A4 = 69 = 440 Hz).
- Effect columns, automation, sample data: out of scope. This reads notes.

Tested against Renoise 3.x files (`doc_version` 66) plus a synthetic fixture.

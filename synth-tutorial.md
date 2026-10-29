# Build a Guitar Synthesizer in Go

This tutorial ports the Real Python article
[Build a Guitar Synthesizer: Play Musical Tablature in Python](https://realpython.com/python-guitar-synthesizer/)
by Bartosz Zaczyński from Python to Go. It follows the same seven steps and
builds the same features. The explanations are rewritten, and the code is
simplified wherever Go allows it.

By the end you'll have:

- the **Karplus-Strong** plucked-string algorithm (explained in detail in [README.md](README.md)),
- **chords** strummed up or down at different speeds,
- **instruments** with their own tunings: acoustic, bass and electric guitar, banjo, ukulele,
- notes read from **scientific pitch notation** (`A4`, `C#3`, …),
- songs laid out in **measures and beats**,
- **effects** (reverb, bass boost, convolution with a real instrument's impulse response),
- a **`play-tab`** command that plays guitar tabs stored in YAML files.

## What's different from the Python version

| Python original                                        | Go version                                                                 |
|--------------------------------------------------------|----------------------------------------------------------------------------|
| Poetry project, `src/digitar` + `src/tablature`         | One Go module with packages `synth`, `audio`, `effects`, `tablature`       |
| NumPy arrays                                           | Plain `[]float64` slices                                                   |
| Feedback loop built from `itertools.cycle` + generator | A simple `for` loop over the output slice                                  |
| Custom `Time` class wrapping `Decimal`                 | `time.Duration` (an exact integer number of nanoseconds)                   |
| `Fraction` for note lengths                            | `synth.Fraction(d, 3, 16)` using integer maths                             |
| Frozen data classes                                    | Structs passed by value                                                     |
| `None` for an unplayed string                          | The constant `synth.Muted` (`-1`); YAML `~` still works                    |
| `@cache` decorators                                    | One `map` cache inside `Synthesizer.Vibrate`                               |
| Pedalboard (effects + MP3 output)                      | ~150 lines of hand-written effects; output is **WAV**                      |
| Pydantic + PyYAML                                      | `gopkg.in/yaml.v3` + a small `validate()` method                           |
| `argparse`                                             | `flag`                                                                     |

> **Output format:** Go's standard library has no MP3 encoder, so everything is
> written as WAV. Convert to MP3 with `ffmpeg -i song.wav song.mp3` if you need to.

---

## Step 1: Set up the project

### Create the module and add the dependency

The repository is already a Go module (`github.com/adambezecny/digitar`). The
only third-party package you'll need is a YAML parser, for Step 7:

```bash
go get gopkg.in/yaml.v3
```

### Project layout

By the end of the tutorial, the repository will look like this. The original
`main.go` from the README stays where it is; everything new lives in its own
packages:

```
digitar/
├── go.mod
├── main.go                 ← the simple Karplus-Strong demo from README.md
├── synth/                  ← sound synthesis (the "digital guitar")
│   ├── burst.go
│   ├── chord.go
│   ├── instrument.go
│   ├── pitch.go
│   ├── processing.go
│   ├── stroke.go
│   ├── synthesis.go
│   ├── temporal.go
│   └── track.go
├── audio/
│   └── wav.go              ← reading and writing WAV files
├── effects/
│   ├── effects.go          ← reverb, EQ, gain, convolution
│   └── fft.go
├── tablature/
│   ├── models.go           ← YAML tab format
│   └── player.go           ← turns a tab into audio
├── cmd/
│   ├── play-chorus/main.go
│   ├── play-diablo/main.go
│   └── play-tab/main.go
├── examples/               ← small throw-away programs used along the way
└── demo/
    ├── ir/                 ← impulse response WAV files (Step 6)
    └── tabs/               ← YAML tabs (Step 7)
```

Run every command in this tutorial from the repository root.

> **About imports:** each file is shown with its *final* import list. If you
> type the code step by step, Go will complain about imports that aren't used
> yet. Let your editor (gopls) or `goimports` add and remove them for you.

### Immutability, the Go way

The Python version uses frozen (immutable) data classes almost everywhere. That
makes objects safe to share and lets them be used as dictionary keys for
caching.

Go gets most of that for free. **Small structs are passed and assigned by
value, so every function works on its own copy.** Structs whose fields are all
comparable (numbers, strings, other such structs) can be used directly as
`map` keys, which is all you need for caching. Only two types in this project
really change over time, the audio track and the timeline, and they're used
through pointers.

### Represent time

Music needs exact timing: a sixteenth note at 75 BPM must last exactly 0.2 s,
however many of them you add up. Python's `float` drifts, so the original
wraps a `Decimal` in a custom `Time` class.

In Go you can use **`time.Duration`**. It's an integer number of nanoseconds,
so adding durations never accumulates rounding errors. You only need a few
helpers:

```go
// synth/temporal.go
package synth

import (
	"math"
	"time"
)

// Hertz is a frequency in cycles per second.
type Hertz = float64

// Seconds converts a (possibly fractional) number of seconds to a Duration.
func Seconds(s float64) time.Duration {
	return time.Duration(math.Round(s * float64(time.Second)))
}

// Milliseconds converts a (possibly fractional) number of milliseconds to a Duration.
func Milliseconds(ms float64) time.Duration {
	return Seconds(ms / 1000)
}

// Fraction returns num/den of the given duration, e.g. Fraction(whole, 3, 16).
// It uses integer arithmetic, so there are no floating-point rounding errors.
func Fraction(d time.Duration, num, den int64) time.Duration {
	return d * time.Duration(num) / time.Duration(den)
}

// NumSamples returns how many audio samples fit into the given duration.
func NumSamples(d time.Duration, sampleRate int) int {
	return int(math.Round(d.Seconds() * float64(sampleRate)))
}
```

For example, `NumSamples(synth.Milliseconds(2), 44100)` returns `88`: two
milliseconds of CD-quality audio is 88 samples.

You'll add a `Timeline` type to this file in Step 5.

---

## Step 2: Model the sound of a vibrating string

### How Karplus-Strong works

This is a quick recap. [README.md](README.md) explains the algorithm in depth.

1. Fill a short **buffer** (the "delay line") with random noise. This is the pluck.
2. Read the buffer in a **loop**. Every value you read goes to the output.
3. After reading a value, replace it with a **damped sum of it and its
   neighbour**. That's a low-pass filter: high frequencies die out first, as on
   a real string.

The buffer length sets the pitch:

```
buffer length = sample rate / frequency        e.g. 44100 / 440 ≈ 100 samples
```

### The initial noise burst

Any signal with lots of different frequencies can serve as the pluck, but
**white noise** works best: it contains every frequency at roughly equal
strength, so the filter has plenty to shape.

Python defines a `Protocol` (an interface) for burst generators. In Go a
**function type** is enough:

```go
// synth/burst.go
package synth

import "math/rand/v2"

// BurstGenerator produces the initial "pluck": numSamples values in [-1, 1].
type BurstGenerator func(numSamples, sampleRate int) []float64

// WhiteNoise is a BurstGenerator that returns uniformly distributed random
// values. It ignores sampleRate, which other generators (e.g. a sawtooth) need.
func WhiteNoise(numSamples, sampleRate int) []float64 {
	samples := make([]float64, numSamples)
	for i := range samples {
		samples[i] = rand.Float64()*2 - 1
	}
	return samples
}
```

You could later write a `Sawtooth` or `Square` function with the same
signature and plug it in without changing anything else.

### The feedback loop

The Python version builds the loop from `itertools.cycle`, a generator
function and `np.fromiter`. In Go you don't need any of that: allocate the
output slice and walk around the buffer with `i % n`, the same as the simple
`main.go` in this repository.

```go
// synth/synthesis.go (first version)
package synth

import (
	"math"
	"time"
)

const AudioCDSampleRate = 44100

// Synthesizer turns notes into audio samples.
type Synthesizer struct {
	Burst      BurstGenerator
	SampleRate int
}

// Vibrate synthesizes a single plucked string with the Karplus-Strong
// algorithm. damping must be in (0, 0.5]; 0.5 is a plain average.
func (s *Synthesizer) Vibrate(frequency Hertz, duration time.Duration, damping float64) []float64 {
	if damping <= 0 || damping > 0.5 {
		panic("damping must be in the range (0, 0.5]")
	}

	// The buffer length sets the pitch: one trip around the loop is one period.
	n := int(math.Round(float64(s.SampleRate) / frequency))
	buffer := s.Burst(n, s.SampleRate)

	samples := make([]float64, NumSamples(duration, s.SampleRate))
	for i := range samples {
		idx := i % n
		samples[i] = buffer[idx]
		// Sum the sample with its neighbour and scale it down: with
		// damping = 0.5 that's an average (low-pass filter); anything
		// below 0.5 also makes the string lose energy faster.
		buffer[idx] = (buffer[idx] + buffer[(idx+1)%n]) * damping
	}
	return Normalize(RemoveDC(samples))
}
```

**`damping` vs. `decay` in README.md:** the README's loop computes
`decay * 0.5 * (a + b)`, and this one computes `(a + b) * damping`. They're the
same formula with `damping = 0.5 * decay`. So `damping = 0.498` here equals
`decay = 0.996` in the README. A value above 0.5 would make the sound louder on
every pass, which is why `Vibrate` refuses it.

### Remove the DC offset and normalize

Two clean-up steps keep the output well-behaved:

- **Remove the DC offset.** If the random burst happens to be mostly positive,
  the whole waveform sits above zero. That wastes volume range and can cause
  clicks. Subtracting the average re-centres it on zero.
- **Normalize.** Scale everything so that the largest sample (the highest
  peak, positive or negative) is exactly ±1. A sample is a cone position, not
  a loudness (see [README.md](README.md)), but making the peaks as big as the
  format allows gives the loudest result that doesn't clip.

```go
// synth/processing.go
package synth

import "math"

// RemoveDC shifts the samples so that their average is zero.
// It modifies the slice in place and returns it.
func RemoveDC(samples []float64) []float64 {
	if len(samples) == 0 {
		return samples
	}
	var sum float64
	for _, s := range samples {
		sum += s
	}
	mean := sum / float64(len(samples))
	for i := range samples {
		samples[i] -= mean
	}
	return samples
}

// Normalize scales the samples so that the largest one (the highest peak,
// positive or negative) is exactly ±1.
// It modifies the slice in place and returns it.
func Normalize(samples []float64) []float64 {
	var peak float64
	for _, s := range samples {
		peak = math.Max(peak, math.Abs(s))
	}
	if peak == 0 {
		return samples // silence: nothing to scale
	}
	for i := range samples {
		samples[i] /= peak
	}
	return samples
}
```

Both functions change the slice **in place** and return it, so you can nest
them: `Normalize(RemoveDC(samples))`.

### Save the result as a WAV file

The Python version hands the samples to Pedalboard to write an MP3. Here, you
move the `writeWAV` function from `main.go` into its own package. You'll add a
WAV *reader* to this file in Step 6.

```go
// audio/wav.go
// Package audio reads and writes WAV files.
package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
)

// WriteWAV saves samples in [-1, 1] as a mono, 16-bit PCM WAV file.
// Values outside that range are clipped.
func WriteWAV(path string, samples []float64, sampleRate int) error {
	var buf bytes.Buffer
	le := binary.LittleEndian
	dataSize := uint32(len(samples) * 2)

	buf.WriteString("RIFF")
	binary.Write(&buf, le, 36+dataSize)
	buf.WriteString("WAVEfmt ")
	binary.Write(&buf, le, uint32(16))           // fmt chunk size
	binary.Write(&buf, le, uint16(1))            // PCM
	binary.Write(&buf, le, uint16(1))            // mono
	binary.Write(&buf, le, uint32(sampleRate))   // samples per second
	binary.Write(&buf, le, uint32(sampleRate*2)) // bytes per second
	binary.Write(&buf, le, uint16(2))            // bytes per sample frame
	binary.Write(&buf, le, uint16(16))           // bits per sample
	buf.WriteString("data")
	binary.Write(&buf, le, dataSize)

	for _, s := range samples {
		s = math.Max(-1, math.Min(1, s))
		binary.Write(&buf, le, int16(s*math.MaxInt16))
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
```

### Play a scale

Now play do-re-mi-fa-sol-la-ti-do, one note after another (**monophonic**):

```go
// examples/monophonic/main.go
package main

import (
	"github.com/adambezecny/digitar/audio"
	"github.com/adambezecny/digitar/synth"
)

func main() {
	frequencies := []synth.Hertz{261.63, 293.66, 329.63, 349.23, 392, 440, 493.88, 523.25}
	duration := synth.Seconds(0.5)
	damping := 0.495

	synthesizer := &synth.Synthesizer{Burst: synth.WhiteNoise, SampleRate: synth.AudioCDSampleRate}

	var samples []float64
	for _, frequency := range frequencies {
		samples = append(samples, synthesizer.Vibrate(frequency, duration, damping)...)
	}
	if err := audio.WriteWAV("monophonic.wav", samples, synthesizer.SampleRate); err != nil {
		panic(err)
	}
}
```

```bash
go run ./examples/monophonic && afplay monophonic.wav
```

Each note is cut off after half a second, and a new note always stops the
previous one. You can only play one note at a time, which is why it's called
monophonic. That's what Step 3 fixes.

---

## Step 3: Strum multiple strings

### Mix notes together

To hear several notes at once (**polyphony**), add their samples together.
`Overlay` does that and can also start each sound a bit later than the
previous one:

```go
// synth/synthesis.go (add this method)

// Overlay mixes sounds together, starting each one delay later than the
// previous one. The sounds may have different lengths.
func (s *Synthesizer) Overlay(sounds [][]float64, delay time.Duration) []float64 {
	delaySamples := NumSamples(delay, s.SampleRate)

	numSamples := 0
	for i, sound := range sounds {
		numSamples = max(numSamples, i*delaySamples+len(sound))
	}

	samples := make([]float64, numSamples)
	for i, sound := range sounds {
		offset := i * delaySamples
		for j, value := range sound {
			samples[offset+j] += value
		}
	}
	return samples
}
```

`Overlay` doesn't normalize. A chord of six strings is louder than a single
note, and that difference should be kept. You normalize **once**, just before
saving the whole song.

Pluck all six open strings of a guitar at exactly the same time:

```go
// examples/polyphonic/main.go
package main

import (
	"github.com/adambezecny/digitar/audio"
	"github.com/adambezecny/digitar/synth"
)

func main() {
	frequencies := []synth.Hertz{329.63, 246.94, 196.00, 146.83, 110.00, 82.41}
	duration := synth.Seconds(3.5)
	damping := 0.499

	synthesizer := &synth.Synthesizer{Burst: synth.WhiteNoise, SampleRate: synth.AudioCDSampleRate}

	var sounds [][]float64
	for _, frequency := range frequencies {
		sounds = append(sounds, synthesizer.Vibrate(frequency, duration, damping))
	}

	// A delay of 0 starts all strings at exactly the same moment.
	samples := synth.Normalize(synthesizer.Overlay(sounds, 0))
	if err := audio.WriteWAV("polyphonic.wav", samples, synthesizer.SampleRate); err != nil {
		panic(err)
	}
}
```

### Strum speed

It already sounds like a chord, but a mechanical one. A real pick hits the
strings **one after another**, a few milliseconds apart. A fast strum sounds
sharp; a slow one is softer. Spread out completely, it becomes an
**arpeggio**.

Add a 40 ms delay between strings, and let the lower (thicker) strings ring a
bit longer:

```go
// examples/arpeggio/main.go
package main

import (
	"github.com/adambezecny/digitar/audio"
	"github.com/adambezecny/digitar/synth"
)

func main() {
	frequencies := []synth.Hertz{329.63, 246.94, 196.00, 146.83, 110.00, 82.41}
	delay := synth.Milliseconds(40)
	damping := 0.499

	synthesizer := &synth.Synthesizer{Burst: synth.WhiteNoise, SampleRate: synth.AudioCDSampleRate}

	var sounds [][]float64
	for i, frequency := range frequencies {
		// Lower (thicker) strings ring a little longer.
		duration := synth.Seconds(3.5 + 0.25*float64(i))
		sounds = append(sounds, synthesizer.Vibrate(frequency, duration, damping))
	}

	samples := synth.Normalize(synthesizer.Overlay(sounds, delay))
	if err := audio.WriteWAV("arpeggio.wav", samples, synthesizer.SampleRate); err != nil {
		panic(err)
	}
}
```

### Strum direction

Guitarists alternate **downstrokes** (from the thickest string down to the
thinnest) and **upstrokes** (the other way). A downstroke hits the bass strings
first and sounds fuller; an upstroke brings out the high strings.

A stroke therefore has a direction and a speed (the delay between strings):

```go
// synth/stroke.go
package synth

import "time"

// Direction is the direction in which the strings are strummed.
type Direction int

const (
	Down Direction = iota
	Up
)

// Velocity describes a stroke: its direction and the delay between
// plucking two neighbouring strings.
type Velocity struct {
	Direction Direction
	Delay     time.Duration
}

// Downstroke returns a downward stroke with the given delay between strings.
func Downstroke(delay time.Duration) Velocity {
	return Velocity{Down, delay}
}

// Upstroke returns an upward stroke with the given delay between strings.
func Upstroke(delay time.Duration) Velocity {
	return Velocity{Up, delay}
}
```

`Direction` with `iota` is Go's usual replacement for Python's `enum.Enum`.
Usage: `synth.Downstroke(synth.Milliseconds(40))`.

To use the direction, the synthesizer needs to know which strings exist and how
they're tuned. That's the next step.

---

## Step 4: Play notes on a virtual guitar

### Frets and semitones

Pressing a string against a fret shortens it, so it vibrates faster. Each fret
raises the pitch by one **semitone**. Twelve semitones make an **octave**,
which doubles the frequency, so one semitone multiplies the frequency by the
twelfth root of two:

```
new frequency = frequency × 2^(semitones / 12)
```

```go
// synth/pitch.go
package synth

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Pitch is the fundamental frequency of a musical note, in Hz.
type Pitch Hertz

// Adjust raises (positive) or lowers (negative) the pitch by whole semitones.
// Twelve semitones make one octave, which doubles the frequency.
func (p Pitch) Adjust(semitones int) Pitch {
	return p * Pitch(math.Pow(2, float64(semitones)/12))
}
```

`Pitch` is just a `float64` with its own methods. You don't need a struct
around it.

A **string** is tuned to a pitch, and pressing fret *n* adjusts that pitch by
*n* semitones (fret 0 is the open string):

```go
// synth/instrument.go
package synth

import (
	"fmt"
	"slices"
	"time"
)

// VibratingString is a single string tuned to a pitch.
type VibratingString struct {
	Pitch Pitch
}

// PressFret returns the pitch you get when you press the given fret.
// Each fret raises the pitch by one semitone; fret 0 is the open string.
func (s VibratingString) PressFret(fret int) Pitch {
	return s.Pitch.Adjust(fret)
}
```

The A string (110 Hz) pressed at the 12th fret gives 220 Hz, one octave higher:

```
-12: 55.00 Hz      0: 110.00 Hz      1: 116.54 Hz      2: 123.47 Hz
  7: 164.81 Hz    12: 220.00 Hz     24: 440.00 Hz
```

### Scientific pitch notation

Musicians don't write "110 Hz", they write **A2**: the note letter, an optional
`#` (sharp, one semitone up), and the octave number. Octaves start at C, and
**A4 = 440 Hz** is the reference pitch.

To convert a note to a frequency, count how many semitones it is away from A4,
then use the same formula as above. A4 is `4×12 + 9 = 57` semitones above C0:

```go
// synth/pitch.go (add these)

var (
	notationPattern = regexp.MustCompile(`^([A-G]#?)(-?\d+)?$`)
	semitoneNames   = strings.Fields("C C# D D# E F F# G G# A A# B")
)

// ParsePitch reads a note in scientific pitch notation, such as "A4" or "C#3".
// The octave defaults to 0. Only sharps are supported: write A# instead of Bb.
func ParsePitch(notation string) (Pitch, error) {
	match := notationPattern.FindStringSubmatch(notation)
	if match == nil {
		return 0, fmt.Errorf("invalid scientific pitch notation: %q", notation)
	}
	octave := 0
	if match[2] != "" {
		octave, _ = strconv.Atoi(match[2]) // the regexp guarantees a number
	}
	// Distance in semitones from A4 (440 Hz), which is 4*12 + 9 = 57 above C0.
	index := octave*12 + slices.Index(semitoneNames, match[1]) - 57
	return Pitch(440 * math.Pow(2, float64(index)/12)), nil
}
```

```
  C: 16.35 Hz     C0: 16.35 Hz     A#: 29.14 Hz     C#4: 277.18 Hz     A4: 440.00 Hz
```

Flats aren't supported. Write the equivalent sharp instead: B♭ = A#.

### Tune the instrument

Tunings are usually written from the **lowest** string to the highest. A
standard guitar is `E2 A2 D3 G3 B3 E4`. Guitar tabs, however, number strings
the other way: **string 1 is the highest** (E4) and is the top line of the tab.

`StringTuning` stores the strings in tab order. `TuningFromNotes` accepts them
in the usual low-to-high order and reverses them:

```go
// synth/instrument.go (add these)

// StringTuning holds the strings of an instrument, starting with the first
// (highest-pitched) string, the same order as the lines of a guitar tab.
type StringTuning struct {
	Strings []VibratingString
}

// TuningFromNotes builds a tuning from notes listed the usual way, from the
// lowest string to the highest, e.g. "E2", "A2", "D3", "G3", "B3", "E4".
func TuningFromNotes(notes ...string) (StringTuning, error) {
	strings := make([]VibratingString, 0, len(notes))
	for _, note := range slices.Backward(notes) {
		pitch, err := ParsePitch(note)
		if err != nil {
			return StringTuning{}, err
		}
		strings = append(strings, VibratingString{pitch})
	}
	return StringTuning{strings}, nil
}

// MustTuning is like TuningFromNotes but panics on an invalid note.
// It's meant for tunings hard-coded in the program.
func MustTuning(notes ...string) StringTuning {
	tuning, err := TuningFromNotes(notes...)
	if err != nil {
		panic(err)
	}
	return tuning
}
```

`MustTuning` follows the Go convention of `regexp.MustCompile`: use it for
values written in your code, where a typo is a bug, and use `TuningFromNotes`
for input from files.

```go
synth.MustTuning("E2", "A2", "D3", "G3", "B3", "E4") // guitar
synth.MustTuning("E1", "A1", "D2", "G2")             // bass guitar
```

### Chords

A chord is one fret number per string, in tab order. Python uses `None` for a
string you don't play. In Go, a named constant is simpler than a slice of
pointers:

```go
// synth/chord.go
package synth

// Muted marks a string that isn't played at all (written as "x" or "-" in tabs).
const Muted = -1

// Chord holds one fret number per string, starting with the first
// (highest-pitched) string. 0 is an open string, Muted skips the string.
type Chord []int
```

In practice:

| Value      | Meaning                               |
|------------|---------------------------------------|
| `Muted`    | don't play this string                |
| `0`        | play the open string                  |
| `1`, `2`, … | press this fret and play the string   |

For example, `synth.Chord{Muted, Muted, 2, Muted, 0, Muted}` presses the G
string at the 2nd fret (A3, 220 Hz) and plays the open A string (A2, 110 Hz):
the same note, one octave apart.

### A plucked string instrument

What makes a guitar sound different from a banjo? Mainly the **number of strings**,
their **tuning**, and how long they **ring**, which depends on the material:

```go
// synth/instrument.go (add these)

// PluckedStringInstrument describes a guitar, banjo, ukulele, etc.
type PluckedStringInstrument struct {
	Tuning    StringTuning
	Vibration time.Duration // how long each string rings by default
	Damping   float64       // in (0, 0.5]; lower values die out faster
}

// NumStrings returns the number of strings of the instrument.
func (i PluckedStringInstrument) NumStrings() int {
	return len(i.Tuning.Strings)
}

// Upstroke returns the pitches of the chord from the first string to the
// last, i.e. from the highest string to the lowest.
func (i PluckedStringInstrument) Upstroke(chord Chord) []Pitch {
	if len(chord) != i.NumStrings() {
		panic(fmt.Sprintf("chord has %d frets but the instrument has %d strings",
			len(chord), i.NumStrings()))
	}
	var pitches []Pitch
	for n, fret := range chord {
		if fret != Muted {
			pitches = append(pitches, i.Tuning.Strings[n].PressFret(fret))
		}
	}
	return pitches
}

// Downstroke returns the pitches of the chord from the lowest string to the highest.
func (i PluckedStringInstrument) Downstroke(chord Chord) []Pitch {
	pitches := i.Upstroke(chord)
	slices.Reverse(pitches)
	return pitches
}
```

Since the chord is in tab order (highest string first), reading it front to
back is an **upstroke**, and reversing it gives a **downstroke**.

Python caches these two methods with `@cache`. That isn't worth it here,
because computing a few pitches is cheap. The expensive part is `Vibrate`,
which gets a cache below.

Some instruments to try:

| Instrument      | Tuning                     | Vibration | Damping |
|-----------------|----------------------------|-----------|---------|
| Acoustic guitar | `E2 A2 D3 G3 B3 E4`        | 10 s      | 0.498   |
| Bass guitar     | `E1 A1 D2 G2`              | 10 s      | 0.4965  |
| Electric guitar | `E2 A2 D3 G3 B3 E4`        | 0.09 s    | 0.475   |
| Banjo           | `G4 D3 G3 B3 D4`           | 2.5 s     | 0.4965  |
| Ukulele         | `A4 E4 C4 G4`              | 5 s       | 0.498   |

The ukulele notes are deliberately listed from high to low. After
`TuningFromNotes` reverses them, the **last** number of a ukulele chord is the
A string, which matches the chord shapes used in Step 5. For example, C major
`{0, 0, 0, 3}` presses the A string at the 3rd fret.

### Connect the synthesizer to an instrument

Now the synthesizer gets an `Instrument` and a `StrumStrings` method that
plays a whole chord. Here is the final `synth/synthesis.go`. Compared with the
first version, it adds:

- the `Instrument` field and a `NewSynthesizer` constructor,
- `StrumStrings`, which picks the stroke direction, vibrates each pitch and
  overlays the results,
- a **cache** in `Vibrate`. Songs repeat the same notes hundreds of times, and
  without a cache the synthesizer would spend almost all its time recomputing
  them.

```go
// synth/synthesis.go
package synth

import (
	"math"
	"time"
)

const AudioCDSampleRate = 44100

// Synthesizer turns chords played on an instrument into audio samples.
type Synthesizer struct {
	Instrument PluckedStringInstrument
	Burst      BurstGenerator
	SampleRate int

	cache map[vibration][]float64
}

// vibration is the cache key for Vibrate: a struct of comparable fields
// can be used as a map key directly.
type vibration struct {
	frequency Hertz
	duration  time.Duration
	damping   float64
}

// NewSynthesizer returns a synthesizer for the instrument that uses white
// noise at the audio CD sample rate.
func NewSynthesizer(instrument PluckedStringInstrument) *Synthesizer {
	return &Synthesizer{
		Instrument: instrument,
		Burst:      WhiteNoise,
		SampleRate: AudioCDSampleRate,
	}
}

// StrumStrings plays a chord with the given stroke direction and speed.
// Pass vibration = 0 to use the instrument's default vibration time.
func (s *Synthesizer) StrumStrings(chord Chord, velocity Velocity, vibration time.Duration) []float64 {
	if vibration == 0 {
		vibration = s.Instrument.Vibration
	}

	var pitches []Pitch
	if velocity.Direction == Up {
		pitches = s.Instrument.Upstroke(chord)
	} else {
		pitches = s.Instrument.Downstroke(chord)
	}

	sounds := make([][]float64, len(pitches))
	for i, pitch := range pitches {
		sounds[i] = s.Vibrate(Hertz(pitch), vibration, s.Instrument.Damping)
	}
	return s.Overlay(sounds, velocity.Delay)
}

// Vibrate synthesizes a single plucked string with the Karplus-Strong
// algorithm. damping must be in (0, 0.5]; 0.5 is a plain average.
//
// Results are cached because songs repeat the same notes over and over.
// The returned slice is shared with the cache, so don't modify it.
func (s *Synthesizer) Vibrate(frequency Hertz, duration time.Duration, damping float64) []float64 {
	if damping <= 0 || damping > 0.5 {
		panic("damping must be in the range (0, 0.5]")
	}

	key := vibration{frequency, duration, damping}
	if samples, ok := s.cache[key]; ok {
		return samples
	}

	// The buffer length sets the pitch: one trip around the loop is one period.
	n := int(math.Round(float64(s.SampleRate) / frequency))
	buffer := s.Burst(n, s.SampleRate)

	samples := make([]float64, NumSamples(duration, s.SampleRate))
	for i := range samples {
		idx := i % n
		samples[i] = buffer[idx]
		// Sum the sample with its neighbour and scale it down: with
		// damping = 0.5 that's an average (low-pass filter); anything
		// below 0.5 also makes the string lose energy faster.
		buffer[idx] = (buffer[idx] + buffer[(idx+1)%n]) * damping
	}
	Normalize(RemoveDC(samples))

	if s.cache == nil {
		s.cache = make(map[vibration][]float64)
	}
	s.cache[key] = samples
	return samples
}

// Overlay mixes sounds together, starting each one delay later than the
// previous one. The sounds may have different lengths.
func (s *Synthesizer) Overlay(sounds [][]float64, delay time.Duration) []float64 {
	delaySamples := NumSamples(delay, s.SampleRate)

	numSamples := 0
	for i, sound := range sounds {
		numSamples = max(numSamples, i*delaySamples+len(sound))
	}

	samples := make([]float64, numSamples)
	for i, sound := range sounds {
		offset := i * delaySamples
		for j, value := range sound {
			samples[offset+j] += value
		}
	}
	return samples
}
```

Some notes on the Go-specific parts:

- **The cache key is a plain struct.** In Python, `@cache` requires every
  argument to be hashable, so `Overlay`, which takes NumPy arrays, can't be
  cached there. Go has no such rule. You choose what goes into the key, and
  three comparable fields are enough.
- **The cache is created lazily** (`if s.cache == nil`), so a `Synthesizer`
  created with a struct literal, as in the Step 2 and 3 examples, still works.
- **The cached slice is shared.** That's safe because nothing in this project
  modifies a sound after `Vibrate` returns it: `Overlay` and `AddAt` only read
  from it.
- **There's one caveat, which Python has too:** a cached note always reuses the
  same random pluck, so repeated notes sound identical. In practice, the
  chords around them make this hard to notice.
- **`Vibrate` and `Overlay` stay exported.** Python hides them behind a `_`
  prefix once `strum_strings` exists. Here they stay public so the earlier
  examples still compile.

Strum all the open strings of three instruments:

```go
// examples/instruments/main.go
package main

import (
	"fmt"

	"github.com/adambezecny/digitar/audio"
	"github.com/adambezecny/digitar/synth"
)

func main() {
	instruments := map[string]synth.PluckedStringInstrument{
		"acoustic_guitar": {
			Tuning:    synth.MustTuning("E2", "A2", "D3", "G3", "B3", "E4"),
			Vibration: synth.Seconds(10),
			Damping:   0.498,
		},
		"banjo": {
			Tuning:    synth.MustTuning("G4", "D3", "G3", "B3", "D4"),
			Vibration: synth.Seconds(2.5),
			Damping:   0.4965,
		},
		"ukulele": {
			Tuning:    synth.MustTuning("A4", "E4", "C4", "G4"),
			Vibration: synth.Seconds(5),
			Damping:   0.498,
		},
	}

	for name, instrument := range instruments {
		synthesizer := synth.NewSynthesizer(instrument)
		openStrings := make(synth.Chord, instrument.NumStrings()) // all zeros
		samples := synthesizer.StrumStrings(openStrings, synth.Downstroke(synth.Milliseconds(40)), 0)

		filename := name + ".wav"
		if err := audio.WriteWAV(filename, synth.Normalize(samples), synthesizer.SampleRate); err != nil {
			panic(err)
		}
		fmt.Println("Saved file", filename)
	}
}
```

`make(synth.Chord, n)` gives a chord of `n` zeros, which means all strings
open, for free.

---

## Step 5: Compose melodies

### An audio track

A song places many sounds at specific moments, and they often overlap: a chord
keeps ringing while the next one starts. `AudioTrack` is a growing slice that
you can mix sounds into at any point in time:

```go
// synth/track.go
package synth

import "time"

// AudioTrack is a growing sequence of samples that sounds can be mixed into
// at any point in time.
type AudioTrack struct {
	SampleRate int
	Samples    []float64
}

// NewAudioTrack returns an empty track.
func NewAudioTrack(sampleRate int) *AudioTrack {
	return &AudioTrack{SampleRate: sampleRate}
}

// Duration returns the current length of the track.
func (t *AudioTrack) Duration() time.Duration {
	return Seconds(float64(len(t.Samples)) / float64(t.SampleRate))
}

// AddAt mixes samples into the track starting at the given instant. The track
// grows (padded with silence) when the new samples reach past its end.
func (t *AudioTrack) AddAt(instant time.Duration, samples []float64) {
	offset := NumSamples(instant, t.SampleRate)
	if end := offset + len(samples); end > len(t.Samples) {
		t.Samples = append(t.Samples, make([]float64, end-len(t.Samples))...)
	}
	for i, value := range samples {
		t.Samples[offset+i] += value
	}
}
```

The Python version handles three cases separately: a gap after the track, a
full overlap, and a partial overlap. In Go it's one rule: **if the new sound
would end past the track, extend the track with silence first, then add.**
That rule covers all three cases.

### A timeline

A timeline remembers "where we are" in the song. Python overloads the `>>`
operator for this. Go has no operator overloading, so it uses a method:

```go
// synth/temporal.go (add these)

// Timeline tracks the current position in a song.
type Timeline struct {
	Instant time.Duration
}

// Advance moves the timeline forward and returns the new instant.
func (t *Timeline) Advance(d time.Duration) time.Duration {
	t.Instant += d
	return t.Instant
}
```

**Why `Advance` returns the instant (a value) and not the timeline.** Later
you'll write code like this:

```go
[]Stroke{
	{timeline.Advance(0), ...},
	{timeline.Advance(threeSixteenth), ...},
}
```

Go guarantees that **function calls** inside an expression run left to right,
but it doesn't guarantee *when* a plain field read like `timeline.Instant`
happens relative to those calls. Returning the value from the call itself
makes the order unambiguous.

### Play a chorus: "I'm Yours" on a ukulele

The chorus of Jason Mraz's "I'm Yours" uses four chords: **C, G, Am, F**. Each
one is strummed with the same six-stroke pattern, played twice:

| # | Stroke | Speed | Time until next stroke |
|---|--------|-------|------------------------|
| 1 | down   | slow  | 0.65 s |
| 2 | down   | slow  | 0.45 s |
| 3 | up     | slow  | 0.75 s |
| 4 | up     | fast  | 0.20 s |
| 5 | down   | fast  | 0.40 s |
| 6 | up     | slow  | 0.25 s |

The Python version produces this sequence with a generator and
`itertools.cycle`. In Go, just build a slice:

```go
// cmd/play-chorus/main.go
// Command play-chorus plays the chorus of Jason Mraz's "I'm Yours" on a ukulele.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/adambezecny/digitar/audio"
	"github.com/adambezecny/digitar/effects"
	"github.com/adambezecny/digitar/synth"
)

type strum struct {
	interval time.Duration // time until the next strum
	chord    synth.Chord
	velocity synth.Velocity
}

func strummingPattern() []strum {
	chords := []synth.Chord{
		{0, 0, 0, 3}, // C major
		{0, 2, 3, 2}, // G major
		{2, 0, 0, 0}, // A minor
		{2, 0, 1, 0}, // F major
	}

	fast := synth.Milliseconds(10)
	slow := synth.Milliseconds(25)
	strokes := []synth.Velocity{
		synth.Downstroke(slow),
		synth.Downstroke(slow),
		synth.Upstroke(slow),
		synth.Upstroke(fast),
		synth.Downstroke(fast),
		synth.Upstroke(slow),
	}
	intervals := []float64{0.65, 0.45, 0.75, 0.2, 0.4, 0.25}

	var pattern []strum
	for _, chord := range chords {
		for range 2 { // play the pattern twice for each chord
			for i, stroke := range strokes {
				pattern = append(pattern, strum{synth.Seconds(intervals[i]), chord, stroke})
			}
		}
	}
	return pattern
}

func main() {
	ukulele := synth.PluckedStringInstrument{
		Tuning:    synth.MustTuning("A4", "E4", "C4", "G4"),
		Vibration: synth.Seconds(5),
		Damping:   0.498,
	}
	synthesizer := synth.NewSynthesizer(ukulele)
	audioTrack := synth.NewAudioTrack(synthesizer.SampleRate)
	timeline := synth.Timeline{}

	for _, s := range strummingPattern() {
		audioTrack.AddAt(timeline.Instant, synthesizer.StrumStrings(s.chord, s.velocity, 0))
		timeline.Advance(s.interval)
	}

	samples := applyEffects(audioTrack, "demo/ir/ukulele.wav", 15)
	if err := audio.WriteWAV("chorus.wav", synth.Normalize(samples), audioTrack.SampleRate); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Saved file chorus.wav")
}

func applyEffects(track *synth.AudioTrack, impulseResponse string, gainDB float64) []float64 {
	chain := []effects.Effect{effects.Reverb()}
	if conv, err := effects.Convolution(impulseResponse, 0.95); err == nil {
		chain = append(chain, conv)
	} else {
		fmt.Fprintln(os.Stderr, "skipping convolution:", err)
	}
	chain = append(chain,
		effects.LowShelfFilter(440, 10, 1),
		effects.Gain(gainDB),
	)
	return effects.Chain(chain...)(track.Samples, track.SampleRate)
}
```

This listing already includes the effects from Step 6. If you're following
along step by step, start with
`audio.WriteWAV("chorus.wav", synth.Normalize(audioTrack.Samples), ...)` and
add `applyEffects` later.

```bash
go run ./cmd/play-chorus && afplay chorus.wav
```

The stroke times in the table were estimated by ear. That works, but it's
fragile, and you can't change the tempo. Real music is written in **measures**
and **beats**.

### Measures, beats and note lengths

- The **beat** is the steady pulse you tap your foot to.
- A **measure** (or bar) groups a fixed number of beats.
- The **time signature** says how many beats a measure has (top number) and
  which note length counts as one beat (bottom number). In **4/4**, a measure
  has four beats and each beat is a quarter note.
- The **tempo**, in beats per minute (BPM), turns all of this into seconds.

At **75 BPM in 4/4**:

```
beat       = 60 s / 75   = 0.8 s
measure    = 4 beats     = 3.2 s
whole note = 4 × beat    = 3.2 s   (4 = the bottom number of 4/4)
```

Every other note length is a fraction of the whole note:

| Note            | Fraction                  | At 75 BPM |
|-----------------|---------------------------|-----------|
| whole           | 1                         | 3.2 s     |
| seven-sixteenth | 7/16 (= ¼ + ⅛ + ¹⁄₁₆)     | 1.4 s     |
| five-sixteenth  | 5/16 (= ¼ + ¹⁄₁₆)         | 1.0 s     |
| three-sixteenth | 3/16 (= ⅛ + ¹⁄₁₆)         | 0.6 s     |
| eighth          | 1/8                       | 0.4 s     |
| sixteenth       | 1/16                      | 0.2 s     |
| thirty-second   | 1/32                      | 0.1 s     |

In Go this is integer arithmetic on `time.Duration`. `synth.Fraction(whole, 3, 16)`
is exactly 600 ms, with no `Fraction` or `Decimal` type needed.

### A timeline that knows about measures

When reading a tab, you often need to **jump to the start of the next
measure**, even if the notes in the current one ended earlier. `MeasuredTimeline`
extends `Timeline` (through struct embedding) and remembers where the last
measure ended:

```go
// synth/temporal.go (add these)

// MeasuredTimeline is a Timeline that also knows where measures begin and end.
// The zero value starts at 0 s; set Measure before calling NextMeasure.
type MeasuredTimeline struct {
	Timeline
	Measure            time.Duration
	lastMeasureEndedAt time.Duration
}

// NextMeasure jumps to the start of the next measure, even if the current
// one isn't over yet, and returns the new instant.
func (t *MeasuredTimeline) NextMeasure() time.Duration {
	if t.Measure <= 0 {
		panic("measure duration must be positive")
	}
	t.lastMeasureEndedAt += t.Measure
	t.Instant = t.lastMeasureEndedAt
	return t.Instant
}
```

Because of embedding, `MeasuredTimeline` also has `Advance` and `Instant`.
`Measure` can be changed at any time, for songs whose time signature changes.

```go
timeline := &synth.MeasuredTimeline{Measure: synth.Seconds(3.2)}
timeline.Advance(synth.Seconds(0.6))
timeline.Advance(synth.Seconds(0.8)) // 1.4 s
timeline.NextMeasure()               // 3.2 s
timeline.Measure = synth.Seconds(2)
timeline.NextMeasure()               // 5.2 s
```

### How to read guitar tablature

A **guitar tab** draws one horizontal line per string, with string 1 (the
highest, E4) at the top. A number on a line is the fret to press on that
string. Numbers stacked in the same column are played together as a chord.

```
E4 |-0-----0-------|-0-----0-------7-|
B3 |-0--0--0-------|-1-----1---------|
G3 |-2--2--2-------|-2--2--2---------|
D3 |-2-----2-------|-4-----4---------|
A2 |-0-----0-------|-0-----0---------|
E2 |---------------|-----------------|
```

Tabs show *what* to play but usually not *how long* each note lasts. Sites
such as [Songsterr](https://www.songsterr.com/) add rhythm symbols under the
tab and a player you can listen to. You'll encode the rhythm explicitly as
note offsets.

### Play the Diablo theme

Here are the first two measures of the **Tristram** theme from *Diablo* (by
Matt Uelmen), at 75 BPM in 4/4. Each stroke records exactly what the tab says:
when, which frets, and how to strum.

```go
// cmd/play-diablo/main.go
// Command play-diablo plays the first measures of the Tristram theme from Diablo.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/adambezecny/digitar/audio"
	"github.com/adambezecny/digitar/effects"
	"github.com/adambezecny/digitar/synth"
)

const (
	beatsPerMinute  = 75
	beatsPerMeasure = 4
	noteValue       = 4 // the bottom number of the 4/4 time signature
)

var (
	beat    = time.Minute / beatsPerMinute
	measure = beat * beatsPerMeasure

	wholeNote       = beat * noteValue
	sevenSixteenth  = synth.Fraction(wholeNote, 7, 16)
	fiveSixteenth   = synth.Fraction(wholeNote, 5, 16)
	threeSixteenth  = synth.Fraction(wholeNote, 3, 16)
	oneEighth       = synth.Fraction(wholeNote, 1, 8)
	oneSixteenth    = synth.Fraction(wholeNote, 1, 16)
	oneThirtySecond = synth.Fraction(wholeNote, 1, 32)

	slow      = synth.Milliseconds(40)
	fast      = synth.Milliseconds(20)
	superFast = synth.Milliseconds(5)
)

// x marks a string that isn't played, just like in a printed tab.
const x = synth.Muted

// Stroke is one strum exactly as it appears in the tab.
type Stroke struct {
	Instant  time.Duration
	Chord    synth.Chord
	Velocity synth.Velocity
}

func main() {
	acousticGuitar := synth.PluckedStringInstrument{
		Tuning:    synth.MustTuning("E2", "A2", "D3", "G3", "B3", "E4"),
		Vibration: synth.Seconds(10),
		Damping:   0.498,
	}
	synthesizer := synth.NewSynthesizer(acousticGuitar)
	audioTrack := synth.NewAudioTrack(synthesizer.SampleRate)
	timeline := &synth.MeasuredTimeline{Measure: measure}

	for _, strokes := range measures(timeline) {
		for _, stroke := range strokes {
			audioTrack.AddAt(stroke.Instant,
				synthesizer.StrumStrings(stroke.Chord, stroke.Velocity, 0))
		}
	}
	save(audioTrack, "diablo.wav")
}

func measures(timeline *synth.MeasuredTimeline) [][]Stroke {
	return [][]Stroke{
		measure01(timeline),
		measure02(timeline),
	}
}

func measure01(timeline *synth.MeasuredTimeline) []Stroke {
	return []Stroke{
		{timeline.Advance(0), synth.Chord{0, 0, 2, 2, 0, x}, synth.Downstroke(slow)},
		{timeline.Advance(threeSixteenth), synth.Chord{x, 0, 2, x, x, x}, synth.Upstroke(fast)},
		{timeline.Advance(oneEighth), synth.Chord{0, 0, 2, 2, 0, x}, synth.Downstroke(slow)},
	}
}

func measure02(timeline *synth.MeasuredTimeline) []Stroke {
	return []Stroke{
		{timeline.NextMeasure(), synth.Chord{0, 4, 2, 1, 0, x}, synth.Downstroke(slow)},
		{timeline.Advance(threeSixteenth), synth.Chord{x, x, 2, x, x, x}, synth.Downstroke(superFast)},
		{timeline.Advance(oneEighth), synth.Chord{0, 4, 2, 1, 0, x}, synth.Downstroke(slow)},
		{timeline.Advance(sevenSixteenth), synth.Chord{7, x, x, x, x, x}, synth.Downstroke(superFast)},
	}
}

func save(track *synth.AudioTrack, filename string) {
	samples := synth.Normalize(applyEffects(track))
	if err := audio.WriteWAV(filename, samples, track.SampleRate); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Saved file", filename)
}

func applyEffects(track *synth.AudioTrack) []float64 {
	chain := []effects.Effect{effects.Reverb()}
	if conv, err := effects.Convolution("demo/ir/acoustic.wav", 0.95); err == nil {
		chain = append(chain, conv)
	} else {
		fmt.Fprintln(os.Stderr, "skipping convolution:", err)
	}
	chain = append(chain,
		effects.LowShelfFilter(440, 10, 1),
		effects.Gain(6),
	)
	return effects.Chain(chain...)(track.Samples, track.SampleRate)
}
```

Things to notice:

- **Only three numbers are inputs** (tempo and time signature). Every duration
  is derived from them, so changing `beatsPerMinute` re-times the whole song.
- **The local constant `x = synth.Muted`** makes chords read like a printed
  tab: `{0, 0, 2, 2, 0, x}`.
- **Each stroke advances the timeline** by the length of the *previous* note.
  `measure02` starts with `NextMeasure()` to jump to the start of bar 2.
- **The Python `class MeasureTiming` / `class Note` namespaces** become plain
  package-level variables. Go doesn't complain about unused package-level
  variables, so you can keep the full set of note lengths for later measures.
- As with the chorus, the effects come from Step 6.

```bash
go run ./cmd/play-diablo && afplay diablo.wav
```

The full song has 78 measures. Writing them all as Go code would be tedious,
which is why Step 7 moves tabs into data files.

---

## Step 6: Effects for more realism

The raw synthesizer sounds a bit thin and dry, because a real guitar has a
wooden **body** and is played in a **room**. The Python version uses Spotify's
Pedalboard library for this. Go has no such library, but the effects you need
are short enough to write yourself.

An effect is a function from samples to samples, and a "pedalboard" is just a
chain of them:

```go
// effects/effects.go
// Package effects implements a few simple audio effects, similar to the
// guitar pedals of Spotify's Pedalboard library.
package effects

import (
	"fmt"
	"math"

	"github.com/adambezecny/digitar/audio"
)

// Effect processes samples and returns the result.
type Effect func(samples []float64, sampleRate int) []float64

// Chain applies effects one after another, like pedals on a pedalboard.
func Chain(effects ...Effect) Effect {
	return func(samples []float64, sampleRate int) []float64 {
		for _, effect := range effects {
			samples = effect(samples, sampleRate)
		}
		return samples
	}
}
```

### Gain

Volume in decibels: +6 dB roughly doubles the amplitude, and every +20 dB
multiplies it by 10.

```go
// Gain changes the volume by the given number of decibels.
func Gain(db float64) Effect {
	return func(samples []float64, _ int) []float64 {
		factor := math.Pow(10, db/20)
		out := make([]float64, len(samples))
		for i, s := range samples {
			out[i] = s * factor
		}
		return out
	}
}
```

### Boost the bass with a low-shelf filter

A guitar body resonates in the low frequencies. A **low-shelf filter** boosts
everything below a cutoff frequency, here +10 dB below 440 Hz. This is a
standard *biquad* filter: each output sample is a weighted mix of the last two
inputs and the last two outputs. The weights come from the well-known "Audio EQ
Cookbook" formulas.

```go
// LowShelfFilter boosts (or cuts) all frequencies below cutoffHz by gainDB.
// It's a standard biquad filter from the "Audio EQ Cookbook" by R. Bristow-Johnson.
func LowShelfFilter(cutoffHz, gainDB, q float64) Effect {
	return func(samples []float64, sampleRate int) []float64 {
		a := math.Pow(10, gainDB/40)
		w0 := 2 * math.Pi * cutoffHz / float64(sampleRate)
		cos := math.Cos(w0)
		alpha := math.Sin(w0) / (2 * q)
		sq := 2 * math.Sqrt(a) * alpha

		b0 := a * ((a + 1) - (a-1)*cos + sq)
		b1 := 2 * a * ((a - 1) - (a+1)*cos)
		b2 := a * ((a + 1) - (a-1)*cos - sq)
		a0 := (a + 1) + (a-1)*cos + sq
		a1 := -2 * ((a - 1) + (a+1)*cos)
		a2 := (a + 1) + (a-1)*cos - sq

		out := make([]float64, len(samples))
		var x1, x2, y1, y2 float64 // previous inputs and outputs
		for i, x := range samples {
			y := (b0*x + b1*x1 + b2*x2 - a1*y1 - a2*y2) / a0
			x2, x1 = x1, x
			y2, y1 = y1, y
			out[i] = y
		}
		return out
	}
}
```

### Reverb

Reverb is the dense cloud of echoes a room adds to a sound. The classic
**Schroeder reverb** imitates it with:

- four **comb filters** in parallel. Each is a feedback echo with a slightly
  different delay (about 32 to 37 ms), standing in for walls at different
  distances.
- two **allpass filters** in series. They smear the separate echoes into a
  smooth tail without changing the overall tone.

The result is mixed with the original ("dry") signal using the same default
levels as Pedalboard's `Reverb`.

```go
// Reverb simulates a room with a classic Schroeder reverberator: four
// parallel echo loops (comb filters) followed by two allpass filters that
// smear the echoes into a smooth tail.
func Reverb() Effect {
	const (
		feedback = 0.84
		wetLevel = 0.33
		dryLevel = 0.4
	)
	return func(samples []float64, sampleRate int) []float64 {
		scale := func(delay int) int { return delay * sampleRate / 44100 }

		wet := make([]float64, len(samples))
		for _, delay := range []int{1557, 1617, 1491, 1422} {
			comb := make([]float64, len(samples))
			d := scale(delay)
			for i, x := range samples {
				comb[i] = x
				if i >= d {
					comb[i] += feedback * comb[i-d]
				}
				wet[i] += comb[i] * (1 - feedback) / 4
			}
		}
		for _, delay := range []int{556, 441} {
			wet = allpass(wet, scale(delay), 0.5)
		}

		out := make([]float64, len(samples))
		for i := range samples {
			out[i] = dryLevel*samples[i] + wetLevel*wet[i]
		}
		return out
	}
}

func allpass(samples []float64, delay int, gain float64) []float64 {
	out := make([]float64, len(samples))
	for i, x := range samples {
		out[i] = -gain * x
		if i >= delay {
			out[i] += samples[i-delay] + gain*out[i-delay]
		}
	}
	return out
}
```

### Convolution with an impulse response

This effect makes the biggest difference. An **impulse response (IR)** is a
recording of how a real space, or a real instrument body, responds to a single
short click. **Convolving** your dry signal with that recording makes it sound
as if it had been played through that guitar body or in that room.

Conceptually, convolution places a copy of the IR at every sample, scaled by
that sample's value, and adds all the copies up. Done naively, that's
`len(song) × len(IR)` multiplications: for a one-minute song and a one-second
IR, about 10¹¹ operations. The standard trick is to use the **FFT**. Convolution
in time equals multiplication of spectra, which takes about a second instead
of hours.

```go
// Convolution imprints the sound of a real instrument or room, recorded as an
// impulse response (IR) in a WAV file, onto the samples. mix is the share of
// the processed ("wet") signal, from 0 to 1.
func Convolution(impulseResponseFile string, mix float64) (Effect, error) {
	ir, irRate, err := audio.ReadWAV(impulseResponseFile)
	if err != nil {
		return nil, err
	}
	return func(samples []float64, sampleRate int) []float64 {
		if irRate != sampleRate {
			panic(fmt.Sprintf("%s: sample rate %d Hz doesn't match %d Hz",
				impulseResponseFile, irRate, sampleRate))
		}
		wet := convolve(samples, ir)
		// Match the loudness of the wet signal to the dry one before mixing.
		gain := peak(samples) / max(peak(wet), 1e-12)
		out := make([]float64, len(samples))
		for i := range samples {
			out[i] = (1-mix)*samples[i] + mix*gain*wet[i]
		}
		return out
	}, nil
}

func peak(samples []float64) float64 {
	var p float64
	for _, s := range samples {
		p = max(p, math.Abs(s))
	}
	return p
}
```

The FFT is a textbook radix-2 implementation. It's self-contained, so there's
no extra dependency:

```go
// effects/fft.go
package effects

import (
	"math"
	"math/cmplx"
)

// convolve returns the first len(x) samples of the convolution of x and h.
// It multiplies their spectra (via the FFT) instead of sliding h over x,
// which would take hours for a song and a one-second impulse response.
func convolve(x, h []float64) []float64 {
	n := 1
	for n < len(x)+len(h)-1 {
		n <<= 1
	}
	a := make([]complex128, n)
	b := make([]complex128, n)
	for i, v := range x {
		a[i] = complex(v, 0)
	}
	for i, v := range h {
		b[i] = complex(v, 0)
	}

	fft(a, false)
	fft(b, false)
	for i := range a {
		a[i] *= b[i]
	}
	fft(a, true)

	out := make([]float64, len(x))
	for i := range out {
		out[i] = real(a[i])
	}
	return out
}

// fft is an in-place iterative radix-2 fast Fourier transform.
// len(a) must be a power of two. inverse=true computes the inverse transform.
func fft(a []complex128, inverse bool) {
	n := len(a)

	// Reorder the elements by bit-reversed index.
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}

	// Combine ever larger blocks ("butterflies").
	for size := 2; size <= n; size <<= 1 {
		angle := 2 * math.Pi / float64(size)
		if !inverse {
			angle = -angle
		}
		step := cmplx.Rect(1, angle)
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := range size / 2 {
				u := a[start+k]
				v := a[start+k+size/2] * w
				a[start+k] = u + v
				a[start+k+size/2] = u - v
				w *= step
			}
		}
	}

	if inverse {
		for i := range a {
			a[i] /= complex(float64(n), 0)
		}
	}
}
```

### Read the impulse response WAV file

Impulse responses are usually distributed as 24-bit or 32-bit float WAV files,
often in stereo. Add a reader to `audio/wav.go` that handles the common formats
and mixes all channels down to mono:

```go
// audio/wav.go (add this)

// ReadWAV loads a PCM (8/16/24/32-bit) or 32-bit float WAV file and mixes all
// channels down to mono. It returns samples in [-1, 1] and the sample rate.
func ReadWAV(path string) ([]float64, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("%s: not a WAV file", path)
	}

	le := binary.LittleEndian
	var format, channels, bits uint16
	var sampleRate uint32
	var pcm []byte

	// A WAV file is a list of chunks: 4-byte ID, 4-byte size, then the payload.
	for pos := 12; pos+8 <= len(data); {
		id := string(data[pos : pos+4])
		size := int(le.Uint32(data[pos+4 : pos+8]))
		body := data[pos+8 : min(pos+8+size, len(data))]
		switch id {
		case "fmt ":
			format = le.Uint16(body[0:2])
			channels = le.Uint16(body[2:4])
			sampleRate = le.Uint32(body[4:8])
			bits = le.Uint16(body[14:16])
			if format == 0xFFFE && len(body) >= 26 { // WAVE_FORMAT_EXTENSIBLE
				format = le.Uint16(body[24:26])
			}
		case "data":
			pcm = body
		}
		pos += 8 + size + size%2 // chunks are padded to an even size
	}
	if channels == 0 || pcm == nil {
		return nil, 0, fmt.Errorf("%s: missing fmt or data chunk", path)
	}

	width := int(bits / 8)
	decode := func(b []byte) float64 {
		switch {
		case format == 3 && bits == 32:
			return float64(math.Float32frombits(le.Uint32(b)))
		case format == 1 && bits == 8:
			return (float64(b[0]) - 128) / 128
		case format == 1 && bits == 16:
			return float64(int16(le.Uint16(b))) / (1 << 15)
		case format == 1 && bits == 24:
			v := int32(b[0]) | int32(b[1])<<8 | int32(int8(b[2]))<<16
			return float64(v) / (1 << 23)
		case format == 1 && bits == 32:
			return float64(int32(le.Uint32(b))) / (1 << 31)
		}
		return math.NaN()
	}
	if math.IsNaN(decode(make([]byte, width))) {
		return nil, 0, errors.New(path + ": unsupported WAV sample format")
	}

	frameSize := width * int(channels)
	samples := make([]float64, len(pcm)/frameSize)
	for i := range samples {
		frame := pcm[i*frameSize:]
		var sum float64
		for c := range int(channels) {
			sum += decode(frame[c*width:])
		}
		samples[i] = sum / float64(channels)
	}
	return samples, int(sampleRate), nil
}
```

The decoder doesn't resample. Use impulse responses recorded at 44.1 kHz, or
convert them first, for example with
`ffmpeg -i in.wav -ar 44100 out.wav`.

### Where to get impulse responses

Put them in `demo/ir/` as `acoustic.wav`, `bass.wav`, `electric.wav`,
`banjo.wav` and `ukulele.wav`. Some sources:

- The original article's downloadable materials include a ready-made, renamed set.
- [Open AIR](https://www.openair.hosted.york.ac.uk/) has impulse responses of
  real rooms and halls.
- Free guitar-body and cabinet IR packs are easy to find online.

**The demos still work without them.** `play-chorus` and `play-diablo` print
`skipping convolution: …` and apply the remaining effects.

### Apply the effects

The `applyEffects` functions in `cmd/play-chorus` and `cmd/play-diablo` (shown
in Step 5) build this chain:

```
Reverb → Convolution(IR, mix 0.95) → LowShelfFilter(440 Hz, +10 dB, Q 1) → Gain(+6 dB, or +15 dB for the ukulele)
```

Then normalize and save. Listen to the result before and after: the guitar
should sound warmer, rounder, and as if it's in a room.

Electric and bass guitars follow the same recipe. Use a different tuning, a
short `Vibration` for the electric guitar's palm-muted "chug", and an amp or
cabinet impulse response.

---

## Step 7: Load tabs from a file

Hard-coding every measure in Go doesn't scale. In this step you'll describe
songs in **YAML** files and write a `play-tab` command that plays them.

### The file format

YAML suits this well. It's easy to edit by hand, it's hierarchical, and its
**anchors** (`&name`) and **aliases** (`*name`) let you define a repeated
chord or a whole measure once and reuse it. Tabs are full of repetition.

```yaml
title: Hello, World!            # optional
artist: John Doe                # optional
tracks:
  acoustic:                     # any name you like
    url: https://www.songsterr.com/hello   # optional
    weight: 0.8                 # optional: volume in the final mix, default 1.0
    instrument:
      tuning: [E2, A2, D3, G3, B3, E4]
      vibration: 5.5            # seconds
      damping: 0.498            # optional: default 0.5
      effects:                  # optional, applied in order
      - Reverb
      - Convolution:
          impulse_response_filename: acoustic.wav   # relative to this file
          mix: 0.95
    tablature:
      beats_per_minute: 75
      measures:
      - time_signature: 4/4
        notes:                  # optional: a measure may be empty
        - frets: [0, 0, 2, 2, 0, ~]   # ~ = string not played
          offset: 1/8           # optional: wait this fraction of a whole note first
          upstroke: true        # optional: default false (downstroke)
          arpeggio: 0.04        # optional: seconds between strings, default 0.005
          vibration: 3.5        # optional: overrides the instrument's vibration
      - time_signature: 4/4     # an empty measure
      - time_signature: 4/4
        notes: &loop            # anchor: remember this list of notes as "loop"
        - frets: &seven [~, ~, ~, ~, 7, ~]
        - frets: *seven         # alias: reuse the frets defined above
          offset: 1/4
        - frets: *seven
          offset: 1/4
      - time_signature: 4/4
        notes: *loop            # reuse the whole measure
```

The format is identical to the Python version's, so tabs written for that
version also play here. The only exception is Pedalboard effects that haven't
been ported: they're skipped with a warning.

### Models

Python uses Pydantic models. In Go, structs with `yaml` tags do the same job:

```go
// tablature/models.go
// Package tablature loads guitar tabs from YAML files and plays them.
package tablature

import (
	"fmt"
	"os"
	"regexp"

	"github.com/adambezecny/digitar/synth"
	"gopkg.in/yaml.v3"
)

const (
	DefaultStringDamping   = 0.5
	DefaultArpeggioSeconds = 0.005
	DefaultTrackWeight     = 1.0
)

var fractionPattern = regexp.MustCompile(`^\d+/\d+$`)

// Song is the root element of a tab file.
type Song struct {
	Title  string            `yaml:"title"`
	Artist string            `yaml:"artist"`
	Tracks map[string]*Track `yaml:"tracks"`
}

// Track is one instrument and the tab it plays.
type Track struct {
	URL        string     `yaml:"url"`
	Weight     float64    `yaml:"weight"`
	Instrument Instrument `yaml:"instrument"`
	Tablature  Tablature  `yaml:"tablature"`
}

// Instrument mirrors synth.PluckedStringInstrument plus a chain of effects.
type Instrument struct {
	Tuning    []string `yaml:"tuning"`
	Vibration float64  `yaml:"vibration"`
	Damping   float64  `yaml:"damping"`
	// Each effect is either a name ("Reverb") or a single-key map from the
	// name to its parameters ({"Gain": {"gain_db": 6}}).
	Effects []any `yaml:"effects"`
}

// Tablature is the tempo and the measures of one track.
type Tablature struct {
	BeatsPerMinute int       `yaml:"beats_per_minute"`
	Measures       []Measure `yaml:"measures"`
}

// Measure has a time signature such as "4/4" and the notes played in it.
type Measure struct {
	TimeSignature string `yaml:"time_signature"`
	Notes         []Note `yaml:"notes"`
}

// Note is a single note or chord.
type Note struct {
	Frets     []*int  `yaml:"frets"`     // nil (~ in YAML) = muted string
	Offset    string  `yaml:"offset"`    // fraction of the whole note, e.g. "1/8"
	Upstroke  bool    `yaml:"upstroke"`  // strum upward instead of downward
	Arpeggio  float64 `yaml:"arpeggio"`  // seconds between plucking neighbouring strings
	Vibration float64 `yaml:"vibration"` // seconds; 0 = instrument default
}
```

`Frets` is a `[]*int` because YAML's `~` (null) must be distinguishable from
fret `0`. It's converted to a `synth.Chord` with `synth.Muted` when the tab is
played.

#### Default values

Pydantic fills in defaults for missing fields. In Go you get the same result
with a small, idiomatic trick: **`yaml.v3` leaves a field untouched when it's
missing from the file**, so you set the defaults first and then decode on top
of them:

```go
// tablature/models.go (continued)

// The UnmarshalYAML methods fill in default values for fields that are
// missing from the file: yaml.v3 leaves fields it doesn't find untouched.
// The local "plain" type has the same fields but no methods, which stops
// Decode from calling UnmarshalYAML again in an endless loop.

func (t *Track) UnmarshalYAML(value *yaml.Node) error {
	type plain Track
	p := plain{Weight: DefaultTrackWeight}
	err := value.Decode(&p)
	*t = Track(p)
	return err
}

func (i *Instrument) UnmarshalYAML(value *yaml.Node) error {
	type plain Instrument
	p := plain{Damping: DefaultStringDamping}
	err := value.Decode(&p)
	*i = Instrument(p)
	return err
}

func (n *Note) UnmarshalYAML(value *yaml.Node) error {
	type plain Note
	p := plain{Offset: "0/1", Arpeggio: DefaultArpeggioSeconds}
	err := value.Decode(&p)
	*n = Note(p)
	return err
}
```

YAML anchors and aliases need no extra code: `yaml.v3` resolves them before
calling these methods.

#### Loading and validation

Pydantic validates fields with type annotations like `PositiveFloat` or
`conlist(..., min_length=1)`. In Go, a plain `validate()` method checks the
same rules and returns a helpful error:

```go
// tablature/models.go (continued)

// LoadSong reads and validates a tab file.
func LoadSong(path string) (*Song, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var song Song
	if err := yaml.Unmarshal(data, &song); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := song.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &song, nil
}

func (s *Song) validate() error {
	if len(s.Tracks) == 0 {
		return fmt.Errorf("no tracks")
	}
	for name, track := range s.Tracks {
		if err := track.validate(); err != nil {
			return fmt.Errorf("track %q: %w", name, err)
		}
	}
	return nil
}

func (t *Track) validate() error {
	if t.Weight < 0 {
		return fmt.Errorf("weight must not be negative")
	}
	if len(t.Instrument.Tuning) == 0 {
		return fmt.Errorf("tuning needs at least one string")
	}
	for _, note := range t.Instrument.Tuning {
		if _, err := synth.ParsePitch(note); err != nil {
			return err
		}
	}
	if t.Instrument.Vibration <= 0 {
		return fmt.Errorf("vibration must be positive")
	}
	if t.Instrument.Damping <= 0 || t.Instrument.Damping > 0.5 {
		return fmt.Errorf("damping must be in the range (0, 0.5]")
	}
	if t.Tablature.BeatsPerMinute <= 0 {
		return fmt.Errorf("beats_per_minute must be positive")
	}

	for i, measure := range t.Tablature.Measures {
		if !fractionPattern.MatchString(measure.TimeSignature) {
			return fmt.Errorf("measure %d: invalid time signature %q", i+1, measure.TimeSignature)
		}
		for _, note := range measure.Notes {
			if len(note.Frets) != len(t.Instrument.Tuning) {
				return fmt.Errorf("measure %d: incorrect number of frets", i+1)
			}
			for _, fret := range note.Frets {
				if fret != nil && *fret < 0 {
					return fmt.Errorf("measure %d: negative fret number", i+1)
				}
			}
			if !fractionPattern.MatchString(note.Offset) {
				return fmt.Errorf("measure %d: invalid offset %q", i+1, note.Offset)
			}
			if note.Arpeggio < 0 || note.Vibration < 0 {
				return fmt.Errorf("measure %d: arpeggio and vibration must not be negative", i+1)
			}
		}
	}
	return nil
}

// BeatsPerMeasure is the top number of the time signature.
func (m Measure) BeatsPerMeasure() int {
	beats, _ := parseFraction(m.TimeSignature)
	return int(beats)
}

// NoteValue is the bottom number of the time signature: 4 means that one
// beat lasts a quarter note.
func (m Measure) NoteValue() int {
	_, value := parseFraction(m.TimeSignature)
	return int(value)
}

// parseFraction splits a validated "num/den" string into two numbers.
func parseFraction(s string) (num, den int64) {
	fmt.Sscanf(s, "%d/%d", &num, &den)
	return num, den
}
```

Because validation runs before anything is synthesized, the `panic` calls in
`synth` (such as a chord/string count mismatch) can't be triggered by a bad
file. You get an error like this instead:

```
error: song.yaml: track "acoustic": measure 3: incorrect number of frets
```

### The player

The player does by hand what you did in `play-diablo`, but driven by the data:

1. For each track, build a synthesizer for its instrument.
2. **`read`** walks the measures. It sets the measure length from the time
   signature, advances by each note's offset, strums, and then jumps to the
   next measure.
3. Apply the track's effects.
4. Mix the tracks, weighted and padded to the longest one, then normalize and
   save.

```go
// tablature/player.go
package tablature

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adambezecny/digitar/audio"
	"github.com/adambezecny/digitar/effects"
	"github.com/adambezecny/digitar/synth"
)

const SampleRate = synth.AudioCDSampleRate

// Play synthesizes every track of the tab file, mixes them and saves the
// result as a WAV file.
func Play(path, output string) error {
	song, err := LoadSong(path)
	if err != nil {
		return err
	}

	// Relative paths in the tab (e.g. impulse responses) are relative to it.
	baseDir := filepath.Dir(path)

	var mix []float64
	for name, track := range song.Tracks {
		samples, err := synthesize(track, baseDir)
		if err != nil {
			return fmt.Errorf("track %q: %w", name, err)
		}
		// Pad the shorter of the two so that they have equal length.
		if len(samples) > len(mix) {
			mix = append(mix, make([]float64, len(samples)-len(mix))...)
		}
		for i, s := range samples {
			mix[i] += track.Weight * s
		}
	}

	if err := audio.WriteWAV(output, synth.Normalize(mix), SampleRate); err != nil {
		return err
	}
	fmt.Println("Saved file", output)
	return nil
}

func synthesize(track *Track, baseDir string) ([]float64, error) {
	tuning, err := synth.TuningFromNotes(track.Instrument.Tuning...)
	if err != nil {
		return nil, err
	}
	synthesizer := synth.NewSynthesizer(synth.PluckedStringInstrument{
		Tuning:    tuning,
		Vibration: synth.Seconds(track.Instrument.Vibration),
		Damping:   track.Instrument.Damping,
	})
	audioTrack := synth.NewAudioTrack(SampleRate)
	read(track.Tablature, synthesizer, audioTrack, &synth.MeasuredTimeline{})

	effect, err := chain(track.Instrument.Effects, baseDir)
	if err != nil {
		return nil, err
	}
	return effect(audioTrack.Samples, SampleRate), nil
}

// read places every note of the tab on the timeline and synthesizes it.
func read(tab Tablature, synthesizer *synth.Synthesizer, audioTrack *synth.AudioTrack, timeline *synth.MeasuredTimeline) {
	beat := time.Minute / time.Duration(tab.BeatsPerMinute)
	for _, measure := range tab.Measures {
		timeline.Measure = beat * time.Duration(measure.BeatsPerMeasure())
		wholeNote := beat * time.Duration(measure.NoteValue())
		for _, note := range measure.Notes {
			num, den := parseFraction(note.Offset)
			instant := timeline.Advance(synth.Fraction(wholeNote, num, den))

			stroke := synth.Downstroke
			if note.Upstroke {
				stroke = synth.Upstroke
			}
			audioTrack.AddAt(instant, synthesizer.StrumStrings(
				toChord(note.Frets),
				stroke(synth.Seconds(note.Arpeggio)),
				synth.Seconds(note.Vibration),
			))
		}
		timeline.NextMeasure()
	}
}

// toChord converts YAML frets, where nil (~) means a muted string.
func toChord(frets []*int) synth.Chord {
	chord := make(synth.Chord, len(frets))
	for i, fret := range frets {
		if fret == nil {
			chord[i] = synth.Muted
		} else {
			chord[i] = *fret
		}
	}
	return chord
}

// chain builds the effects listed in the tab. Unknown effects are skipped
// with a warning, so tabs written for Pedalboard still play.
func chain(config []any, baseDir string) (effects.Effect, error) {
	var chain []effects.Effect
	for _, item := range config {
		name, params, err := parseEffect(item)
		if err != nil {
			return nil, err
		}
		number := func(key string, fallback float64) float64 {
			switch v := params[key].(type) {
			case int:
				return float64(v)
			case float64:
				return v
			}
			return fallback
		}

		switch name {
		case "Reverb":
			chain = append(chain, effects.Reverb())
		case "Gain":
			chain = append(chain, effects.Gain(number("gain_db", 0)))
		case "LowShelfFilter":
			chain = append(chain, effects.LowShelfFilter(
				number("cutoff_frequency_hz", 440), number("gain_db", 0), number("q", 0.7071)))
		case "Convolution":
			file, _ := params["impulse_response_filename"].(string)
			conv, err := effects.Convolution(filepath.Join(baseDir, file), number("mix", 1))
			if err != nil {
				return nil, err
			}
			chain = append(chain, conv)
		default:
			fmt.Fprintf(os.Stderr, "warning: skipping unsupported effect %q\n", name)
		}
	}
	return effects.Chain(chain...), nil
}

// parseEffect accepts either "Name" or {"Name": {param: value, ...}}.
func parseEffect(item any) (string, map[string]any, error) {
	switch v := item.(type) {
	case string:
		return v, nil, nil
	case map[string]any:
		if len(v) == 1 {
			for name, params := range v {
				p, _ := params.(map[string]any)
				return name, p, nil
			}
		}
	}
	return "", nil, fmt.Errorf("invalid effect: %v", item)
}
```

Differences from the Python player:

- **No temporary `chdir`.** The Python version changes the working directory
  so that relative IR paths resolve. Here, `filepath.Join(baseDir, file)`
  resolves them relative to the tab file directly, without side effects.
- **No dynamic class lookup.** Python creates effects with
  `getattr(pedalboard, class_name)(**params)`. Go maps the supported names
  explicitly in a `switch`. That's a bit more typing, but every supported
  effect and parameter is visible in one place.
- **`synth.Downstroke` and `synth.Upstroke` are ordinary functions**, so
  `stroke := synth.Downstroke` stores one in a variable, just like Python's
  `Velocity.down`.
- **Padding and mixing happen in one loop.** The Python version does
  `pad_to_longest` and `np.sum` as separate steps.
- **Track order doesn't matter.** Go's map iteration order is random, but
  addition is commutative, so the mix comes out the same.

### The command

```go
// cmd/play-tab/main.go
// Command play-tab synthesizes a guitar tab stored in a YAML file.
//
// Usage:
//
//	play-tab [-o output.wav] tab.yaml
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adambezecny/digitar/tablature"
)

func main() {
	output := flag.String("o", "", "output WAV file (default: <tab name>.wav)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: play-tab [-o output.wav] tab.yaml")
		os.Exit(2)
	}

	path := flag.Arg(0)
	if *output == "" {
		name := filepath.Base(path)
		*output = strings.TrimSuffix(name, filepath.Ext(name)) + ".wav"
	}

	if err := tablature.Play(path, *output); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

All the logic lives in the `tablature` package, so `main` only parses flags.
This replaces Poetry's `[tool.poetry.scripts]` entry point.

### Try it

Here is the Diablo intro from Step 5 as a tab file, with a bass guitar added.
Notice how anchors (`&am`, `&e`, `&bass`) avoid repetition:

```yaml
# demo/tabs/diablo-intro.yaml
title: Tristram (first two measures)
artist: Matt Uelmen
tracks:
  acoustic:
    instrument:
      tuning: [E2, A2, D3, G3, B3, E4]
      vibration: 10
      damping: 0.498
      effects:
      - Reverb
      - Convolution:
          impulse_response_filename: ../ir/acoustic.wav
          mix: 0.95
      - LowShelfFilter:
          cutoff_frequency_hz: 440
          gain_db: 10
          q: 1
      - Gain:
          gain_db: 6
    tablature:
      beats_per_minute: 75
      measures:
      - time_signature: 4/4
        notes:
        - frets: &am [0, 0, 2, 2, 0, ~]
          arpeggio: 0.04
        - frets: [~, 0, 2, ~, ~, ~]
          offset: 3/16
          upstroke: true
          arpeggio: 0.02
        - frets: *am
          offset: 1/8
          arpeggio: 0.04
      - time_signature: 4/4
        notes:
        - frets: &e [0, 4, 2, 1, 0, ~]
          arpeggio: 0.04
        - frets: [~, ~, 2, ~, ~, ~]
          offset: 3/16
        - frets: *e
          offset: 1/8
          arpeggio: 0.04
        - frets: [7, ~, ~, ~, ~, ~]
          offset: 7/16
  bass:
    weight: 0.6
    instrument:
      tuning: [E1, A1, D2, G2]
      vibration: 10
      damping: 0.4965
    tablature:
      beats_per_minute: 75
      measures:
      - time_signature: 4/4
        notes: &bass
        - frets: [~, ~, 0, ~]
        - frets: [~, ~, 0, ~]
          offset: 1/2
      - time_signature: 4/4
        notes: *bass
```

```bash
go run ./cmd/play-tab demo/tabs/diablo-intro.yaml && afplay diablo-intro.wav
```

To install the command so you can call it from anywhere:

```bash
go install ./cmd/play-tab
```

The Convolution effect in this tab needs `demo/ir/acoustic.wav`. Unlike the
demo programs, `play-tab` stops with an error if a listed IR file is missing,
because the tab explicitly asks for it. Remove the `Convolution` entry if you
don't have one.

---

## Conclusion

You've built a complete plucked-string synthesizer in Go:

- the **Karplus-Strong** algorithm in a dozen lines,
- **polyphonic** chords with adjustable **strum speed and direction**,
- **instruments and tunings** defined from scientific pitch notation,
- songs laid out on a **timeline of measures and beats** with exact integer timing,
- hand-written **effects**: gain, low-shelf EQ, Schroeder reverb, FFT convolution,
- a **YAML tab format** with anchors, defaults and validation, and a
  `play-tab` command.

Apart from `gopkg.in/yaml.v3`, everything uses only the Go standard library.

### Next steps

- **Make it sound more human:** add small random variations to the timing,
  damping and strum speed, and render in stereo with each string panned
  slightly differently.
- **Playing techniques:** bends, slides, hammer-ons, pull-offs and palm muting
  all change the pitch or damping while a string is ringing.
- **More instruments:** mandolin, bouzouki, harp, sitar, oud. Most of them only
  need a new tuning, vibration time and impulse response. With bigger changes
  to the algorithm you can even model drums (see the drum variant in
  [README.md](README.md)) or bowed strings.
- **More formats:** write MP3 or FLAC directly, or import ASCII tabs.
- **Write your own tabs** for your favourite songs, for example from
  [Songsterr](https://www.songsterr.com/).

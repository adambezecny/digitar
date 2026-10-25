# digitar

A tiny Go program that synthesizes plucked-guitar sounds using the
**Karplus-Strong algorithm** and writes them to a `.wav` file.
It uses only the Go standard library.

## Quick start

```bash
go run main.go
afplay pluck.wav   # macOS; use any audio player elsewhere
```

This plays a short C-major arpeggio (C4, E4, G4, C5), with 0.6 seconds per note.

---

## Background: what is pitch?

**Pitch is how high or low a sound seems to you.** A bird's chirp is high and
a bass drum is low.

Sound is air vibrating back and forth. **Pitch depends on how many times per
second the vibration repeats.** That count is the **frequency**, measured in
**hertz (Hz)**.

- **Fast vibration means high pitch.** A whistle is around 2000 Hz.
- **Slow vibration means low pitch.** A bass guitar's lowest string is around 41 Hz.
- People can hear roughly **20 Hz to 20,000 Hz**.

```
Low pitch (slow repeats):    /‾‾‾‾‾‾\______/‾‾‾‾‾‾\______/‾‾‾

High pitch (fast repeats):   /\/\/\/\/\/\/\/\/\/\/\/\/\/\/\/\
```

Each musical note is a specific frequency:

| Note                           | Frequency |
|--------------------------------|-----------|
| C4 (middle C)                  | 261.63 Hz |
| A4 (the orchestra tuning note) | 440 Hz    |
| A5 (one octave above A4)       | 880 Hz    |

**Doubling the frequency raises the note by one octave.** It sounds like "the
same note, but higher."

Frequency is physical and can be measured. Pitch is what your brain perceives
from it. In practice the two words are usually used interchangeably.

---

## The Karplus-Strong algorithm in simple words

Picture a guitar string. When you pluck it, it starts out vibrating
chaotically. Within a few milliseconds that chaos settles into a clear pitch,
and the sound then slowly fades, with the high, bright frequencies dying out
faster than the low ones.

Karplus-Strong (Kevin Karplus and Alex Strong, 1983) copies this with a simple
trick:

1. **Fill a buffer with random noise.** This is the pluck, the initial chaotic
   energy in the string.
2. **Loop over the buffer in a circle.** On each step, output the current
   sample, then replace it with **the average of it and the next sample**,
   multiplied by a decay factor slightly below 1.
3. **Repeat.**

```
  ┌────────────────────────────────────────────┐
  │                                            │
  ▼                                            │
[ delay line of N samples ] ──► output ──► avg(x[i], x[i+1]) × decay
```

### Why it works

- **Looping a buffer of length N repeats the same pattern every N samples.**
  Anything that repeats has a pitch, so `frequency = sampleRate / N`. A longer
  buffer gives a lower note, like a longer string.
- **Averaging neighbouring samples is a low-pass filter.** It smooths out
  sharp, high-frequency jaggedness, so harsh noise turns into a mellow tone.
  High frequencies get smoothed away fastest, just as on a real string.
- **The decay factor** (for example `0.996`) makes the whole sound fade out
  over time, like a real string losing energy to the air.

Put simply, it is noise in a loop that slowly smooths itself, and the result
sounds like a string.

### How the buffer length sets the pitch

```go
n := int(math.Round(sampleRate / freq)) // e.g. 44100 / 440 ≈ 100 samples
```

If the pattern repeats every 100 samples at 44,100 samples per second, it
repeats 441 times per second. You hear roughly the note A4.

- A **shorter buffer** repeats more often and gives a **higher pitch**.
- A **longer buffer** repeats less often and gives a **lower pitch**.

The random noise sets the tone colour, which is why it sounds like a string and
not a flute. The **loop length sets the pitch**.

### What actually repeats?

**The wave shape stored in the buffer repeats.** After the "pluck", the buffer
holds 100 random values. The loop reads them from start to end, wraps around
and reads the buffer again, so the speaker receives roughly the same jagged
shape every 100 samples:

```
 one cycle (100 samples)      next cycle               next cycle
|  /\    /\/\   _/\         |  /\    /\/\   _/\         |  /\    /\/\   _/\
| /  \/\/    \_/   \/\      | /  \/\/    \_/   \/\      | /  \/\/    \_/   \/\
 ◄──── 2.27 ms ────►
```

Your ear doesn't need a smooth sine wave to hear a note. **Any shape that
repeats regularly is heard as a pitch**:

- **How often it repeats** (441 times per second) sets the **pitch**.
- **The shape of one cycle** sets the **timbre**, the character of the sound.
  That's why a violin and a guitar playing the same A4 still sound different.

On a real guitar, the "pattern" is the shape of the bent string travelling to
the bridge and back. The buffer is that round trip: **one trip along the string
and back.**

### But aren't the values being smoothed?

Yes. The loop visits the **same slots** over and over, but the **values** in
those slots change on every visit:

```go
out[i] = buf[idx]                                // 1. read the current value
buf[idx] = decay * 0.5 * (buf[idx] + buf[next])  // 2. overwrite it with a smoothed value
```

One cycle later the loop comes back to this slot and reads the **smoothed**
value, not the original one.

#### Tiny example

Here is a buffer of **4 slots** (instead of 100), with `decay` ignored:

```
buf = [ 1.0, -1.0,  0.6, -0.2 ]
```

**Cycle 1** outputs each value, then replaces it with the average of itself and
its right neighbour:

| i | slot | output   | new value                 |
|---|------|----------|---------------------------|
| 0 | 0    | **1.0**  | (1.0 + -1.0)/2 = **0.0**  |
| 1 | 1    | **-1.0** | (-1.0 + 0.6)/2 = **-0.2** |
| 2 | 2    | **0.6**  | (0.6 + -0.2)/2 = **0.2**  |
| 3 | 3    | **-0.2** | (-0.2 + 0.0)/2 = **-0.1** |

Slot 3's neighbour is slot 0, which was already updated to 0.0.
The buffer is now `[ 0.0, -0.2, 0.2, -0.1 ]`.

**Cycle 2** reads these new values:

| i | slot | output   | new value                  |
|---|------|----------|----------------------------|
| 4 | 0    | **0.0**  | (0.0 + -0.2)/2 = **-0.1**  |
| 5 | 1    | **-0.2** | (-0.2 + 0.2)/2 = **0.0**   |
| 6 | 2    | **0.2**  | (0.2 + -0.1)/2 = **0.05**  |
| 7 | 3    | **-0.1** | (-0.1 + -0.1)/2 = **-0.1** |

```
cycle 1:  1.0  -1.0   0.6  -0.2     ← wild jumps
cycle 2:  0.0  -0.2   0.2  -0.1     ← smaller, smoother
cycle 3:  ...                        ← smaller still
```

#### Pitch and smoothing happen at the same time

- **The cycle length** never changes. It's always `n` slots, so the shape comes
  around again every `n` samples. That timing gives the **pitch**.
- **The values** change a little per cycle, so the shape slowly smooths out and
  fades. That gives the "pluck → ring → fade" character:

```
cycle 1:    /\/\/\_/\/\_/\/   jagged, loud   → bright "twang"
cycle 10:   /‾\_/‾‾\__/‾\_    smoother       → warmer
cycle 100:  /‾‾‾\___/‾‾‾\__   almost a sine  → mellow
cycle 500:  ‾‾‾‾‾‾‾‾‾‾‾‾‾‾   almost flat    → silence
```

In the 4-slot toy example the smoothing looks dramatic. With a real 100-slot
buffer, each slot is averaged with very similar neighbours, so **one cycle looks
almost exactly like the previous one**. Your ear hears a steady 441 Hz pitch
whose tone gets gradually softer.

A good way to think about it: **each cycle is a slightly blurred copy of the
previous one**, like photocopying a photocopy. The picture stays recognizable
from one copy to the next, but it slowly gets softer.

---

## Code walkthrough

The whole algorithm lives in `KarplusStrong` in [main.go](main.go):

```go
func KarplusStrong(freq, duration, decay float64) []float64 {
	// STEP 1: the buffer length sets the pitch.
	n := int(math.Round(sampleRate / freq))
	buf := make([]float64, n)

	// STEP 2: "pluck" by filling the buffer with noise in [-1, 1).
	for i := range buf {
		buf[i] = rand.Float64()*2 - 1
	}

	total := int(duration * sampleRate)
	out := make([]float64, total)

	// STEP 3: walk around the buffer in a circle.
	for i := 0; i < total; i++ {
		idx := i % n        // wraps around: 0,1,...,n-1,0,1,...
		next := (idx + 1) % n

		out[i] = buf[idx]   // a) output the current value

		// b) average with the neighbour (smooths = low-pass filter)
		//    and shrink slightly (decay = fade-out)
		buf[idx] = decay * 0.5 * (buf[idx] + buf[next])
	}
	return out
}
```

| Parameter  | Meaning                                                          |
|------------|------------------------------------------------------------------|
| `freq`     | Pitch of the note in Hz (e.g. 440 = A4)                          |
| `duration` | Length of the sound in seconds                                   |
| `decay`    | Fade speed. `0.98` gives a short pluck, `0.999` a long ring. Must be below 1 |

The rest of the program is plumbing:

- **`writeWAV`** writes a 44-byte WAV header followed by the samples as
  16-bit integers (mono, 44.1 kHz). Samples are clipped to `[-1, 1]` and scaled
  to `[-32767, 32767]`.
- **`main`** generates four notes, joins them and saves `pluck.wav`.

---

## Settings to try

| Change                                               | Effect                                                    |
|------------------------------------------------------|-----------------------------------------------------------|
| `decay` 0.98 → 0.999                                 | Short muted pluck → long ringing sustain                  |
| Initial buffer of only `+1`/`-1` values               | Brighter, harsher attack                                  |
| Initial buffer of a smooth wave or low-passed noise  | Softer pluck, like a thumb instead of a pick              |
| Randomly flip the sign of the averaged sample         | Drum-like sound (the original paper's "drum" variant)     |

### Drum variant

Replace the update line in the loop with:

```go
avg := decay * 0.5 * (buf[idx] + buf[next])
if rand.Float64() < 0.5 {
	avg = -avg
}
buf[idx] = avg
```

---

## Known limitation: tuning

The buffer length `n` must be a whole number, so the pitch is slightly off,
and the error grows for high notes. At 44.1 kHz, A5 (880 Hz) needs
`n = 50.1`, which rounds to 50 and gives 882 Hz. The averaging step also adds
about half a sample of delay, so the real pitch is closer to
`sampleRate / (n + 0.5)`.

Production synthesizers fix this with a **fractional delay**, using an
all-pass filter or linear interpolation between samples. For learning and
experimenting, the simple version is good enough.

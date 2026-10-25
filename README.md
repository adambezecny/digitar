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

## Why does it sound like a plucked string?

The algorithm copies, in a very simplified way, the **three physical things
that happen when you pluck a string**. Other instruments behave differently in
at least one of them.

### 1. The attack is short and noisy, and then nothing more is added

A pluck is one short, chaotic burst of energy: the pick or finger snaps off the
string. After that, **nobody adds any more energy**. The string just rings out
on its own.

In the code:
- The **random noise** in the buffer is the snap of the pluck, with every
  frequency at once.
- **Nothing ever adds energy afterwards**, so the sound can only fade.

Compare other instruments:
- **Violin:** the bow keeps feeding energy, so the note can last as long as the
  player wants.
- **Flute, trumpet:** the player's breath keeps feeding energy, so the note is
  steady.
- **Organ:** air keeps flowing, so the note never fades on its own.

So an attack followed by a free decay already rules out all sustained
instruments.

### 2. Harmonics at whole-number multiples, like a string

**In short:** a string (because its ends are clamped) and the code (because its
loop has a fixed length) can only vibrate at 1×, 2×, 3×… the base frequency.
These frequencies line up into one clear note, which is the sound of a string.
The rest of this section explains why.

#### What a harmonic is

When you pluck a string, it doesn't vibrate in just one way. **It vibrates in
several patterns at once**, stacked on top of each other.

Take the guitar's A string, tuned to **110 Hz**. Both ends are fixed (at the
nut and the bridge), so **every vibration pattern has to keep both ends
still**. Only these patterns fit:

```
1st harmonic (whole string swings)       110 Hz   = 1 × 110
nut |‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾| bridge
    |________________________________|

2nd harmonic (two halves swing)          220 Hz   = 2 × 110
    |‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾\________________|
    |________________/‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾|

3rd harmonic (three thirds swing)        330 Hz   = 3 × 110
    |‾‾‾‾‾‾‾‾‾‾\__________/‾‾‾‾‾‾‾‾‾‾|
    |__________/‾‾‾‾‾‾‾‾‾‾\__________|

4th harmonic (four quarters)             440 Hz   = 4 × 110
...and so on: 550, 660, 770 Hz...
```

A pattern of "one and a half bumps" is impossible, because one end would have
to move, and it's clamped. So **the string can only produce 1×, 2×, 3×, 4×…
the base frequency**. These are the whole-number multiples.

You hear all of them **together as a single note**, A at 110 Hz. The mix of
how loud each harmonic is gives the string its tone.

#### "Two halves swing" doesn't mean only half of the string vibrates

**The whole string is vibrating**, but it moves in two sections that swing in
**opposite directions** at the same time, like a seesaw. The point in the
middle doesn't move at all. That still point is called a **node**.

```
moment 1:   |‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾•________________|
moment 2:   |________________•‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾|
            nut            node             bridge
                      (stays still)
```

The 3rd harmonic has two nodes and three sections, the 4th has three nodes and
four sections, and so on.

A plucked string does **all of these patterns at once**, added together. In
slow motion you'd see one complicated, wobbly shape: the sum of the
whole-string swing, the two-halves swing, the three-thirds swing, and so on.

#### Why half the length means twice the frequency

When you pluck a string, the bend you made **travels along the string** like a
ripple, bounces off the end, and comes back. The ripple's speed depends only on
the string's **tension** and **thickness/weight**, not on its length.

One full vibration is the time the ripple needs to travel **to the far end and
back**:

```
time of one vibration = round-trip distance ÷ speed
frequency             = how many round trips per second
```

With real numbers: a guitar string is about **65 cm** long, and on the A string
the ripple travels at about **143 m/s**.

| Vibrating section            | Round trip         | Time per trip | Frequency   |
|------------------------------|--------------------|---------------|-------------|
| Whole string (65 cm)         | 2 × 65 cm = 1.30 m | ≈ 9.1 ms      | **110 Hz**  |
| Half the string (32.5 cm)    | 2 × 32.5 cm = 0.65 m | ≈ 4.5 ms    | **220 Hz**  |

**Same speed, half the distance, so half the time per trip and twice as many
trips per second.** It's like a runner going back and forth on a track: shorten
the track to half its length and they complete twice as many laps per minute
without running any faster.

Frets use the same idea. Pressing a fret makes the vibrating part shorter, so
the note goes up:

| Where you press    | Vibrating length | Note                       |
|--------------------|------------------|----------------------------|
| open string        | 65 cm            | A, 110 Hz                  |
| 5th fret           | ~48.7 cm (¾)     | D, ~147 Hz                 |
| 12th fret (middle) | 32.5 cm (half)   | A, 220 Hz, one octave up   |

#### Try it on a guitar: natural harmonics

1. Pluck the open A string normally. You hear 110 Hz plus all its harmonics.
2. Now **lightly touch** (don't press) the string exactly above the **12th
   fret**, which is the middle of the string, then pluck it and lift your
   finger.

Your finger forces the middle to stand still. That **kills the 1st harmonic**,
because the 1st harmonic needs the middle to move the most. The 2nd harmonic
survives because its node is already in the middle, so you hear a clear,
bell-like note **one octave higher** (220 Hz).

Touching above the **7th fret** (one third of the way along) leaves the 3rd
harmonic, and above the **5th fret** (one quarter) leaves the 4th.

#### Why the code does the same

The code has no physical string. It has a buffer that **repeats every `n`
samples**, and that rule works just like the clamped ends.

Think of the sound as made of simple sine waves. Which sine waves can be part
of a shape that repeats exactly every `n` samples? **Only the ones that
complete a whole number of cycles in `n` samples.**

Here is a small example with an **8-sample** loop:

```
1 cycle in 8 samples   → fits ✅   (ends where it started)
 0  ↗  ↑  ↘  0  ↘  ↓  ↗ | 0 ...

2 cycles in 8 samples  → fits ✅
 0  ↑  0  ↓  0  ↑  0  ↓ | 0 ...

1.5 cycles in 8 samples → doesn't fit ❌
 0  ↑  ↘  ↓  ↗  ↑  ↘  ↓ | 0 ... ← ends at the bottom but the loop jumps
                                  back to the start at 0, so it breaks
```

A sine that does 1.5 cycles ends **halfway through a cycle**. When the loop
jumps back to the start, the wave would have to jump, so it can't be part of a
cleanly repeating shape.

With the real 100-sample buffer at 44,100 samples per second:

| Cycles per 100 samples | Frequency  | Fits the loop?  |
|------------------------|------------|-----------------|
| 1                      | 441 Hz     | ✅ 1st harmonic |
| 1.5                    | 661.5 Hz   | ❌              |
| 2                      | 882 Hz     | ✅ 2nd harmonic |
| 2.5                    | 1102.5 Hz  | ❌              |
| 3                      | 1323 Hz    | ✅ 3rd harmonic |
| 4                      | 1764 Hz    | ✅ 4th harmonic |

**The `n`-sample loop does the same job as the string's clamped ends.** Both
only allow whole-number multiples.

The buffer is the ripple's round trip, so the length rule works the same way
too. Halving the buffer is exactly like pressing the 12th fret:

| Buffer length `n`     | Round trip time | Frequency |
|-----------------------|-----------------|-----------|
| 400 samples           | 9.1 ms          | ~110 Hz   |
| 200 samples (half)    | 4.5 ms          | ~220 Hz   |
| 100 samples (quarter) | 2.3 ms          | ~441 Hz   |

#### Where the random noise fits in

Any shape can be built by adding together the allowed sine waves at different
strengths. For example:

- **Square wave** = 1× + ⅓ of 3× + ⅕ of 5× + … (odd harmonics only, a hollow,
  clarinet-like sound)
- **Sawtooth wave** = 1× + ½ of 2× + ⅓ of 3× + … (all harmonics, a bright,
  brassy sound)
- **Random noise in the loop** = 1×, 2×, 3×, 4×… each at a **random strength**

So the noise doesn't decide *which* frequencies are present; the loop length
already decided that. The noise only decides **how loud each harmonic starts**.
That's why every run sounds slightly different (a different random pluck) but
is always in tune.

#### Instruments that don't follow this rule

Some instruments vibrate in patterns that **don't** fit whole numbers:

| Instrument                          | Vibration patterns (relative to the lowest)          |
|-------------------------------------|------------------------------------------------------|
| **String** (guitar, piano, violin)  | 1, **2**, **3**, **4**, **5**, 6…                    |
| **Drum head** (ideal round membrane)| 1, **1.59**, **2.14**, **2.30**, **2.65**, 2.92…     |
| **Bell**                            | irregular, e.g. 0.5, 1, 1.2, 1.5, 2, 2.5…            |

When the frequencies are whole-number multiples, they **line up** and your
brain merges them into one clear note. When they're at odd ratios like 1.59 or
2.14, they don't line up. Your brain hears a cluster of tones, which makes the
sound **clangy (bell)** or **thuddy without a clear note (drum)**.

#### Why the drum variant sounds like a drum

The **drum variant** (see [Settings to try](#settings-to-try)) randomly flips
the sign of each new sample:

```go
if rand.Float64() < 0.5 {
	avg = -avg
}
```

Now the buffer **no longer comes back as a smoothed copy of itself** each
cycle. The shape keeps getting scrambled, so the clean repeat is broken.
Without a clean repeat, the energy isn't confined to 441, 882, 1323… anymore.
It spreads across frequencies at irregular spacing, just like a drum head.
Same algorithm, one changed line, and **the pitch disappears and you get a
thud.**

### 3. High frequencies fade faster than low ones

This is the most important part. On a real string, the high harmonics lose
energy quickly (to air friction and the stiffness of the string), while the
fundamental keeps ringing much longer. A pluck therefore sounds **bright at the
start and mellow at the end**.

The averaging step does exactly this:

```go
buf[idx] = decay * 0.5 * (buf[idx] + buf[next])
```

Averaging neighbours smooths out fast wiggles (high frequencies) much more than
slow waves (low frequencies). On every pass:
- the high harmonics shrink a lot,
- the low harmonics barely change.

That's the "bright twang → warm ring → fade" you hear, and your brain
recognises it immediately as a plucked string.

### Summary

| Physical feature        | Plucked string                       | Code                                  |
|-------------------------|--------------------------------------|---------------------------------------|
| Sharp noisy attack      | pick snaps the string                | random noise in the buffer            |
| No added energy         | string rings freely                  | nothing refills the buffer            |
| Whole-number harmonics  | string vibrates in halves, thirds…   | buffer repeats every `n` samples      |
| Highs die out first     | air friction, string stiffness       | averaging neighbours (low-pass)       |
| Overall fade            | energy loss                          | `decay`                               |

### Honestly: it's a generic plucked string, not exactly a guitar

The basic algorithm sounds somewhere between a **harpsichord, harp,
nylon-string guitar and banjo**. It doesn't sound specifically like a guitar,
because a real guitar also has:

- **A wooden body** that resonates and colours the sound (the biggest missing
  piece)
- **The pick position**: plucking near the bridge sounds bright and thin, near
  the neck sounds round
- **The string vibrating in two directions** (up/down and side to side), which
  makes the decay richer
- **String stiffness**, which pushes the higher harmonics slightly out of tune

### Ways to make it sound more like a guitar

1. **Add a body filter:** run the output through a few resonant filters around
   100 to 250 Hz to imitate the wooden box.
2. **Imitate the pick position:** subtract a delayed copy of the starting noise
   (a comb filter), so some harmonics are removed, as with a real pluck.
3. **Make the pluck softer:** smooth the starting noise a bit to sound like a
   finger instead of a pick.
4. **Add slight detuning:** run two strings at nearly the same pitch, which
   sounds like the two vibration directions or a 12-string chorus.

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

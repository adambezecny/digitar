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
//
// This is how a chord is built: one sound per string, mixed into one.
// delay is the strum speed: 0 plucks all strings at the same moment,
// 40 ms is a slow strum where the strings come in one after another.
//
// Example with a delay of 2 samples:
//
//	position:   0   1   2   3   4   5
//	sounds[0]:  1   1   1   1
//	sounds[1]:          2   2   2
//	sounds[2]:                  3   3
//	result:     1   1   3   3   5   3
//
// The result is not normalized and can go beyond ±1 (six strings that each
// reach 1.0 can add up to about 6). That's intended: a chord should be louder
// than a single note. Normalize runs once on the whole song before saving.
func (s *Synthesizer) Overlay(sounds [][]float64, delay time.Duration) []float64 {
	// The code works in samples, not time: at 44,100 Hz, 40 ms = 1,764 samples.
	delaySamples := NumSamples(delay, s.SampleRate)

	// Sound i starts at i*delaySamples and lasts len(sound) samples. The result
	// must be long enough for whichever sound ends last, which isn't always the
	// last one to start, because the sounds can have different lengths.
	numSamples := 0
	for i, sound := range sounds {
		numSamples = max(numSamples, i*delaySamples+len(sound))
	}

	// make fills the slice with zeros: silence, the speaker cone at rest.
	samples := make([]float64, numSamples)

	// Add each sound at its starting position. Adding is how mixing works:
	// each sound wants to move the cone by some amount, and when sounds play
	// together the cone moves by the sum of all of them.
	for i, sound := range sounds {
		offset := i * delaySamples
		for j, value := range sound {
			samples[offset+j] += value
		}
	}
	return samples
}

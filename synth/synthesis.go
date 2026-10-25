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

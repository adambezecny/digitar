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

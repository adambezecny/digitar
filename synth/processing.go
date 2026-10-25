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

// Normalize scales the samples so that the loudest one is exactly ±1.
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

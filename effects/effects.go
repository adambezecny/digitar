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

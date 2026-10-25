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

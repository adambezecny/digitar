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

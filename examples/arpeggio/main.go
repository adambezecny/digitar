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

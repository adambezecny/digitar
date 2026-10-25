package main

import (
	"fmt"

	"github.com/adambezecny/digitar/audio"
	"github.com/adambezecny/digitar/synth"
)

func main() {
	instruments := map[string]synth.PluckedStringInstrument{
		"acoustic_guitar": {
			Tuning:    synth.MustTuning("E2", "A2", "D3", "G3", "B3", "E4"),
			Vibration: synth.Seconds(10),
			Damping:   0.498,
		},
		"banjo": {
			Tuning:    synth.MustTuning("G4", "D3", "G3", "B3", "D4"),
			Vibration: synth.Seconds(2.5),
			Damping:   0.4965,
		},
		"ukulele": {
			Tuning:    synth.MustTuning("A4", "E4", "C4", "G4"),
			Vibration: synth.Seconds(5),
			Damping:   0.498,
		},
	}

	for name, instrument := range instruments {
		synthesizer := synth.NewSynthesizer(instrument)
		openStrings := make(synth.Chord, instrument.NumStrings()) // all zeros
		samples := synthesizer.StrumStrings(openStrings, synth.Downstroke(synth.Milliseconds(40)), 0)

		filename := name + ".wav"
		if err := audio.WriteWAV(filename, synth.Normalize(samples), synthesizer.SampleRate); err != nil {
			panic(err)
		}
		fmt.Println("Saved file", filename)
	}
}

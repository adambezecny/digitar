package main

import (
	"fmt"

	"github.com/adambezecny/digitar/synth"
)

func main() {
	a2 := synth.Pitch(110)
	for _, semitones := range []int{-12, 0, 1, 2, 7, 12, 24} {
		fmt.Printf("%3d: %.2f Hz\n", semitones, a2.Adjust(semitones))
	}
	for _, note := range []string{"C", "C0", "A#", "C#4", "A4"} {
		pitch, err := synth.ParsePitch(note)
		if err != nil {
			panic(err)
		}
		fmt.Printf("%3s: %.2f Hz\n", note, pitch)
	}
	fmt.Println(synth.MustTuning("E2", "A2", "D3", "G3", "B3", "E4"))
}

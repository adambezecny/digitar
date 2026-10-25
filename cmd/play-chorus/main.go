// Command play-chorus plays the chorus of Jason Mraz's "I'm Yours" on a ukulele.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/adambezecny/digitar/audio"
	"github.com/adambezecny/digitar/effects"
	"github.com/adambezecny/digitar/synth"
)

type strum struct {
	interval time.Duration // time until the next strum
	chord    synth.Chord
	velocity synth.Velocity
}

func strummingPattern() []strum {
	chords := []synth.Chord{
		{0, 0, 0, 3}, // C major
		{0, 2, 3, 2}, // G major
		{2, 0, 0, 0}, // A minor
		{2, 0, 1, 0}, // F major
	}

	fast := synth.Milliseconds(10)
	slow := synth.Milliseconds(25)
	strokes := []synth.Velocity{
		synth.Downstroke(slow),
		synth.Downstroke(slow),
		synth.Upstroke(slow),
		synth.Upstroke(fast),
		synth.Downstroke(fast),
		synth.Upstroke(slow),
	}
	intervals := []float64{0.65, 0.45, 0.75, 0.2, 0.4, 0.25}

	var pattern []strum
	for _, chord := range chords {
		for range 2 { // play the pattern twice for each chord
			for i, stroke := range strokes {
				pattern = append(pattern, strum{synth.Seconds(intervals[i]), chord, stroke})
			}
		}
	}
	return pattern
}

func main() {
	ukulele := synth.PluckedStringInstrument{
		Tuning:    synth.MustTuning("A4", "E4", "C4", "G4"),
		Vibration: synth.Seconds(5),
		Damping:   0.498,
	}
	synthesizer := synth.NewSynthesizer(ukulele)
	audioTrack := synth.NewAudioTrack(synthesizer.SampleRate)
	timeline := synth.Timeline{}

	for _, s := range strummingPattern() {
		audioTrack.AddAt(timeline.Instant, synthesizer.StrumStrings(s.chord, s.velocity, 0))
		timeline.Advance(s.interval)
	}

	samples := applyEffects(audioTrack, "demo/ir/ukulele.wav", 15)
	if err := audio.WriteWAV("chorus.wav", synth.Normalize(samples), audioTrack.SampleRate); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Saved file chorus.wav")
}

func applyEffects(track *synth.AudioTrack, impulseResponse string, gainDB float64) []float64 {
	chain := []effects.Effect{effects.Reverb()}
	if conv, err := effects.Convolution(impulseResponse, 0.95); err == nil {
		chain = append(chain, conv)
	} else {
		fmt.Fprintln(os.Stderr, "skipping convolution:", err)
	}
	chain = append(chain,
		effects.LowShelfFilter(440, 10, 1),
		effects.Gain(gainDB),
	)
	return effects.Chain(chain...)(track.Samples, track.SampleRate)
}

// Command play-diablo plays the first measures of the Tristram theme from Diablo.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/adambezecny/digitar/audio"
	"github.com/adambezecny/digitar/effects"
	"github.com/adambezecny/digitar/synth"
)

const (
	beatsPerMinute  = 75
	beatsPerMeasure = 4
	noteValue       = 4 // the bottom number of the 4/4 time signature
)

var (
	beat    = time.Minute / beatsPerMinute
	measure = beat * beatsPerMeasure

	wholeNote       = beat * noteValue
	sevenSixteenth  = synth.Fraction(wholeNote, 7, 16)
	fiveSixteenth   = synth.Fraction(wholeNote, 5, 16)
	threeSixteenth  = synth.Fraction(wholeNote, 3, 16)
	oneEighth       = synth.Fraction(wholeNote, 1, 8)
	oneSixteenth    = synth.Fraction(wholeNote, 1, 16)
	oneThirtySecond = synth.Fraction(wholeNote, 1, 32)

	slow      = synth.Milliseconds(40)
	fast      = synth.Milliseconds(20)
	superFast = synth.Milliseconds(5)
)

// x marks a string that isn't played, just like in a printed tab.
const x = synth.Muted

// Stroke is one strum exactly as it appears in the tab.
type Stroke struct {
	Instant  time.Duration
	Chord    synth.Chord
	Velocity synth.Velocity
}

func main() {
	acousticGuitar := synth.PluckedStringInstrument{
		Tuning:    synth.MustTuning("E2", "A2", "D3", "G3", "B3", "E4"),
		Vibration: synth.Seconds(10),
		Damping:   0.498,
	}
	synthesizer := synth.NewSynthesizer(acousticGuitar)
	audioTrack := synth.NewAudioTrack(synthesizer.SampleRate)
	timeline := &synth.MeasuredTimeline{Measure: measure}

	for _, strokes := range measures(timeline) {
		for _, stroke := range strokes {
			audioTrack.AddAt(stroke.Instant,
				synthesizer.StrumStrings(stroke.Chord, stroke.Velocity, 0))
		}
	}
	save(audioTrack, "diablo.wav")
}

func measures(timeline *synth.MeasuredTimeline) [][]Stroke {
	return [][]Stroke{
		measure01(timeline),
		measure02(timeline),
	}
}

func measure01(timeline *synth.MeasuredTimeline) []Stroke {
	return []Stroke{
		{timeline.Advance(0), synth.Chord{0, 0, 2, 2, 0, x}, synth.Downstroke(slow)},
		{timeline.Advance(threeSixteenth), synth.Chord{x, 0, 2, x, x, x}, synth.Upstroke(fast)},
		{timeline.Advance(oneEighth), synth.Chord{0, 0, 2, 2, 0, x}, synth.Downstroke(slow)},
	}
}

func measure02(timeline *synth.MeasuredTimeline) []Stroke {
	return []Stroke{
		{timeline.NextMeasure(), synth.Chord{0, 4, 2, 1, 0, x}, synth.Downstroke(slow)},
		{timeline.Advance(threeSixteenth), synth.Chord{x, x, 2, x, x, x}, synth.Downstroke(superFast)},
		{timeline.Advance(oneEighth), synth.Chord{0, 4, 2, 1, 0, x}, synth.Downstroke(slow)},
		{timeline.Advance(sevenSixteenth), synth.Chord{7, x, x, x, x, x}, synth.Downstroke(superFast)},
	}
}

func save(track *synth.AudioTrack, filename string) {
	samples := synth.Normalize(applyEffects(track))
	if err := audio.WriteWAV(filename, samples, track.SampleRate); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Saved file", filename)
}

func applyEffects(track *synth.AudioTrack) []float64 {
	chain := []effects.Effect{effects.Reverb()}
	if conv, err := effects.Convolution("demo/ir/acoustic.wav", 0.95); err == nil {
		chain = append(chain, conv)
	} else {
		fmt.Fprintln(os.Stderr, "skipping convolution:", err)
	}
	chain = append(chain,
		effects.LowShelfFilter(440, 10, 1),
		effects.Gain(6),
	)
	return effects.Chain(chain...)(track.Samples, track.SampleRate)
}

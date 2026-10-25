package tablature

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adambezecny/digitar/audio"
	"github.com/adambezecny/digitar/effects"
	"github.com/adambezecny/digitar/synth"
)

const SampleRate = synth.AudioCDSampleRate

// Play synthesizes every track of the tab file, mixes them and saves the
// result as a WAV file.
func Play(path, output string) error {
	song, err := LoadSong(path)
	if err != nil {
		return err
	}

	// Relative paths in the tab (e.g. impulse responses) are relative to it.
	baseDir := filepath.Dir(path)

	var mix []float64
	for name, track := range song.Tracks {
		samples, err := synthesize(track, baseDir)
		if err != nil {
			return fmt.Errorf("track %q: %w", name, err)
		}
		// Pad the shorter of the two so that they have equal length.
		if len(samples) > len(mix) {
			mix = append(mix, make([]float64, len(samples)-len(mix))...)
		}
		for i, s := range samples {
			mix[i] += track.Weight * s
		}
	}

	if err := audio.WriteWAV(output, synth.Normalize(mix), SampleRate); err != nil {
		return err
	}
	fmt.Println("Saved file", output)
	return nil
}

func synthesize(track *Track, baseDir string) ([]float64, error) {
	tuning, err := synth.TuningFromNotes(track.Instrument.Tuning...)
	if err != nil {
		return nil, err
	}
	synthesizer := synth.NewSynthesizer(synth.PluckedStringInstrument{
		Tuning:    tuning,
		Vibration: synth.Seconds(track.Instrument.Vibration),
		Damping:   track.Instrument.Damping,
	})
	audioTrack := synth.NewAudioTrack(SampleRate)
	read(track.Tablature, synthesizer, audioTrack, &synth.MeasuredTimeline{})

	effect, err := chain(track.Instrument.Effects, baseDir)
	if err != nil {
		return nil, err
	}
	return effect(audioTrack.Samples, SampleRate), nil
}

// read places every note of the tab on the timeline and synthesizes it.
func read(tab Tablature, synthesizer *synth.Synthesizer, audioTrack *synth.AudioTrack, timeline *synth.MeasuredTimeline) {
	beat := time.Minute / time.Duration(tab.BeatsPerMinute)
	for _, measure := range tab.Measures {
		timeline.Measure = beat * time.Duration(measure.BeatsPerMeasure())
		wholeNote := beat * time.Duration(measure.NoteValue())
		for _, note := range measure.Notes {
			num, den := parseFraction(note.Offset)
			instant := timeline.Advance(synth.Fraction(wholeNote, num, den))

			stroke := synth.Downstroke
			if note.Upstroke {
				stroke = synth.Upstroke
			}
			audioTrack.AddAt(instant, synthesizer.StrumStrings(
				toChord(note.Frets),
				stroke(synth.Seconds(note.Arpeggio)),
				synth.Seconds(note.Vibration),
			))
		}
		timeline.NextMeasure()
	}
}

// toChord converts YAML frets, where nil (~) means a muted string.
func toChord(frets []*int) synth.Chord {
	chord := make(synth.Chord, len(frets))
	for i, fret := range frets {
		if fret == nil {
			chord[i] = synth.Muted
		} else {
			chord[i] = *fret
		}
	}
	return chord
}

// chain builds the effects listed in the tab. Unknown effects are skipped
// with a warning, so tabs written for Pedalboard still play.
func chain(config []any, baseDir string) (effects.Effect, error) {
	var chain []effects.Effect
	for _, item := range config {
		name, params, err := parseEffect(item)
		if err != nil {
			return nil, err
		}
		number := func(key string, fallback float64) float64 {
			switch v := params[key].(type) {
			case int:
				return float64(v)
			case float64:
				return v
			}
			return fallback
		}

		switch name {
		case "Reverb":
			chain = append(chain, effects.Reverb())
		case "Gain":
			chain = append(chain, effects.Gain(number("gain_db", 0)))
		case "LowShelfFilter":
			chain = append(chain, effects.LowShelfFilter(
				number("cutoff_frequency_hz", 440), number("gain_db", 0), number("q", 0.7071)))
		case "Convolution":
			file, _ := params["impulse_response_filename"].(string)
			conv, err := effects.Convolution(filepath.Join(baseDir, file), number("mix", 1))
			if err != nil {
				return nil, err
			}
			chain = append(chain, conv)
		default:
			fmt.Fprintf(os.Stderr, "warning: skipping unsupported effect %q\n", name)
		}
	}
	return effects.Chain(chain...), nil
}

// parseEffect accepts either "Name" or {"Name": {param: value, ...}}.
func parseEffect(item any) (string, map[string]any, error) {
	switch v := item.(type) {
	case string:
		return v, nil, nil
	case map[string]any:
		if len(v) == 1 {
			for name, params := range v {
				p, _ := params.(map[string]any)
				return name, p, nil
			}
		}
	}
	return "", nil, fmt.Errorf("invalid effect: %v", item)
}

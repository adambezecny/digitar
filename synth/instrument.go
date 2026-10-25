package synth

import (
	"fmt"
	"slices"
	"time"
)

// VibratingString is a single string tuned to a pitch.
type VibratingString struct {
	Pitch Pitch
}

// PressFret returns the pitch you get when you press the given fret.
// Each fret raises the pitch by one semitone; fret 0 is the open string.
func (s VibratingString) PressFret(fret int) Pitch {
	return s.Pitch.Adjust(fret)
}

// StringTuning holds the strings of an instrument, starting with the first
// (highest-pitched) string, the same order as the lines of a guitar tab.
type StringTuning struct {
	Strings []VibratingString
}

// TuningFromNotes builds a tuning from notes listed the usual way, from the
// lowest string to the highest, e.g. "E2", "A2", "D3", "G3", "B3", "E4".
func TuningFromNotes(notes ...string) (StringTuning, error) {
	strings := make([]VibratingString, 0, len(notes))
	for _, note := range slices.Backward(notes) {
		pitch, err := ParsePitch(note)
		if err != nil {
			return StringTuning{}, err
		}
		strings = append(strings, VibratingString{pitch})
	}
	return StringTuning{strings}, nil
}

// MustTuning is like TuningFromNotes but panics on an invalid note.
// It's meant for tunings hard-coded in the program.
func MustTuning(notes ...string) StringTuning {
	tuning, err := TuningFromNotes(notes...)
	if err != nil {
		panic(err)
	}
	return tuning
}

// PluckedStringInstrument describes a guitar, banjo, ukulele, etc.
type PluckedStringInstrument struct {
	Tuning    StringTuning
	Vibration time.Duration // how long each string rings by default
	Damping   float64       // in (0, 0.5]; lower values die out faster
}

// NumStrings returns the number of strings of the instrument.
func (i PluckedStringInstrument) NumStrings() int {
	return len(i.Tuning.Strings)
}

// Upstroke returns the pitches of the chord from the first string to the
// last, i.e. from the highest string to the lowest.
func (i PluckedStringInstrument) Upstroke(chord Chord) []Pitch {
	if len(chord) != i.NumStrings() {
		panic(fmt.Sprintf("chord has %d frets but the instrument has %d strings",
			len(chord), i.NumStrings()))
	}
	var pitches []Pitch
	for n, fret := range chord {
		if fret != Muted {
			pitches = append(pitches, i.Tuning.Strings[n].PressFret(fret))
		}
	}
	return pitches
}

// Downstroke returns the pitches of the chord from the lowest string to the highest.
func (i PluckedStringInstrument) Downstroke(chord Chord) []Pitch {
	pitches := i.Upstroke(chord)
	slices.Reverse(pitches)
	return pitches
}

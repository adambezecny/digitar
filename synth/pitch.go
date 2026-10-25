package synth

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Pitch is the fundamental frequency of a musical note, in Hz.
type Pitch Hertz

var (
	notationPattern = regexp.MustCompile(`^([A-G]#?)(-?\d+)?$`)
	semitoneNames   = strings.Fields("C C# D D# E F F# G G# A A# B")
)

// ParsePitch reads a note in scientific pitch notation, such as "A4" or "C#3".
// The octave defaults to 0. Only sharps are supported: write A# instead of Bb.
func ParsePitch(notation string) (Pitch, error) {
	match := notationPattern.FindStringSubmatch(notation)
	if match == nil {
		return 0, fmt.Errorf("invalid scientific pitch notation: %q", notation)
	}
	octave := 0
	if match[2] != "" {
		octave, _ = strconv.Atoi(match[2]) // the regexp guarantees a number
	}
	// Distance in semitones from A4 (440 Hz), which is 4*12 + 9 = 57 above C0.
	index := octave*12 + slices.Index(semitoneNames, match[1]) - 57
	return Pitch(440 * math.Pow(2, float64(index)/12)), nil
}

// Adjust raises (positive) or lowers (negative) the pitch by whole semitones.
// Twelve semitones make one octave, which doubles the frequency.
func (p Pitch) Adjust(semitones int) Pitch {
	return p * Pitch(math.Pow(2, float64(semitones)/12))
}

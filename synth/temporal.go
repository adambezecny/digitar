package synth

import (
	"math"
	"time"
)

// Hertz is a frequency in cycles per second.
type Hertz = float64

// Seconds converts a (possibly fractional) number of seconds to a Duration.
func Seconds(s float64) time.Duration {
	return time.Duration(math.Round(s * float64(time.Second)))
}

// Milliseconds converts a (possibly fractional) number of milliseconds to a Duration.
func Milliseconds(ms float64) time.Duration {
	return Seconds(ms / 1000)
}

// Fraction returns num/den of the given duration, e.g. Fraction(whole, 3, 16).
// It uses integer arithmetic, so there are no floating-point rounding errors.
func Fraction(d time.Duration, num, den int64) time.Duration {
	return d * time.Duration(num) / time.Duration(den)
}

// NumSamples returns how many audio samples fit into the given duration.
func NumSamples(d time.Duration, sampleRate int) int {
	return int(math.Round(d.Seconds() * float64(sampleRate)))
}

// Timeline tracks the current position in a song.
type Timeline struct {
	Instant time.Duration
}

// Advance moves the timeline forward and returns the new instant.
func (t *Timeline) Advance(d time.Duration) time.Duration {
	t.Instant += d
	return t.Instant
}

// MeasuredTimeline is a Timeline that also knows where measures begin and end.
// The zero value starts at 0 s; set Measure before calling NextMeasure.
type MeasuredTimeline struct {
	Timeline
	Measure            time.Duration
	lastMeasureEndedAt time.Duration
}

// NextMeasure jumps to the start of the next measure, even if the current
// one isn't over yet, and returns the new instant.
func (t *MeasuredTimeline) NextMeasure() time.Duration {
	if t.Measure <= 0 {
		panic("measure duration must be positive")
	}
	t.lastMeasureEndedAt += t.Measure
	t.Instant = t.lastMeasureEndedAt
	return t.Instant
}

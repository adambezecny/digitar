package synth

import "time"

// Direction is the direction in which the strings are strummed.
type Direction int

const (
	Down Direction = iota
	Up
)

// Velocity describes a stroke: its direction and the delay between
// plucking two neighbouring strings.
type Velocity struct {
	Direction Direction
	Delay     time.Duration
}

// Downstroke returns a downward stroke with the given delay between strings.
func Downstroke(delay time.Duration) Velocity {
	return Velocity{Down, delay}
}

// Upstroke returns an upward stroke with the given delay between strings.
func Upstroke(delay time.Duration) Velocity {
	return Velocity{Up, delay}
}

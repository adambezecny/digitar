package synth

// Muted marks a string that isn't played at all (written as "x" or "-" in tabs).
const Muted = -1

// Chord holds one fret number per string, starting with the first
// (highest-pitched) string. 0 is an open string, Muted skips the string.
type Chord []int

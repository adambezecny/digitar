// Package tablature loads guitar tabs from YAML files and plays them.
package tablature

import (
	"fmt"
	"os"
	"regexp"

	"github.com/adambezecny/digitar/synth"
	"gopkg.in/yaml.v3"
)

const (
	DefaultStringDamping   = 0.5
	DefaultArpeggioSeconds = 0.005
	DefaultTrackWeight     = 1.0
)

var fractionPattern = regexp.MustCompile(`^\d+/\d+$`)

// Song is the root element of a tab file.
type Song struct {
	Title  string            `yaml:"title"`
	Artist string            `yaml:"artist"`
	Tracks map[string]*Track `yaml:"tracks"`
}

// Track is one instrument and the tab it plays.
type Track struct {
	URL        string     `yaml:"url"`
	Weight     float64    `yaml:"weight"`
	Instrument Instrument `yaml:"instrument"`
	Tablature  Tablature  `yaml:"tablature"`
}

// Instrument mirrors synth.PluckedStringInstrument plus a chain of effects.
type Instrument struct {
	Tuning    []string `yaml:"tuning"`
	Vibration float64  `yaml:"vibration"`
	Damping   float64  `yaml:"damping"`
	// Each effect is either a name ("Reverb") or a single-key map from the
	// name to its parameters ({"Gain": {"gain_db": 6}}).
	Effects []any `yaml:"effects"`
}

// Tablature is the tempo and the measures of one track.
type Tablature struct {
	BeatsPerMinute int       `yaml:"beats_per_minute"`
	Measures       []Measure `yaml:"measures"`
}

// Measure has a time signature such as "4/4" and the notes played in it.
type Measure struct {
	TimeSignature string `yaml:"time_signature"`
	Notes         []Note `yaml:"notes"`
}

// Note is a single note or chord.
type Note struct {
	Frets     []*int  `yaml:"frets"`     // nil (~ in YAML) = muted string
	Offset    string  `yaml:"offset"`    // fraction of the whole note, e.g. "1/8"
	Upstroke  bool    `yaml:"upstroke"`  // strum upward instead of downward
	Arpeggio  float64 `yaml:"arpeggio"`  // seconds between plucking neighbouring strings
	Vibration float64 `yaml:"vibration"` // seconds; 0 = instrument default
}

// The UnmarshalYAML methods fill in default values for fields that are
// missing from the file: yaml.v3 leaves fields it doesn't find untouched.
// The local "plain" type has the same fields but no methods, which stops
// Decode from calling UnmarshalYAML again in an endless loop.

func (t *Track) UnmarshalYAML(value *yaml.Node) error {
	type plain Track
	p := plain{Weight: DefaultTrackWeight}
	err := value.Decode(&p)
	*t = Track(p)
	return err
}

func (i *Instrument) UnmarshalYAML(value *yaml.Node) error {
	type plain Instrument
	p := plain{Damping: DefaultStringDamping}
	err := value.Decode(&p)
	*i = Instrument(p)
	return err
}

func (n *Note) UnmarshalYAML(value *yaml.Node) error {
	type plain Note
	p := plain{Offset: "0/1", Arpeggio: DefaultArpeggioSeconds}
	err := value.Decode(&p)
	*n = Note(p)
	return err
}

// LoadSong reads and validates a tab file.
func LoadSong(path string) (*Song, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var song Song
	if err := yaml.Unmarshal(data, &song); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := song.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &song, nil
}

func (s *Song) validate() error {
	if len(s.Tracks) == 0 {
		return fmt.Errorf("no tracks")
	}
	for name, track := range s.Tracks {
		if err := track.validate(); err != nil {
			return fmt.Errorf("track %q: %w", name, err)
		}
	}
	return nil
}

func (t *Track) validate() error {
	if t.Weight < 0 {
		return fmt.Errorf("weight must not be negative")
	}
	if len(t.Instrument.Tuning) == 0 {
		return fmt.Errorf("tuning needs at least one string")
	}
	for _, note := range t.Instrument.Tuning {
		if _, err := synth.ParsePitch(note); err != nil {
			return err
		}
	}
	if t.Instrument.Vibration <= 0 {
		return fmt.Errorf("vibration must be positive")
	}
	if t.Instrument.Damping <= 0 || t.Instrument.Damping > 0.5 {
		return fmt.Errorf("damping must be in the range (0, 0.5]")
	}
	if t.Tablature.BeatsPerMinute <= 0 {
		return fmt.Errorf("beats_per_minute must be positive")
	}

	for i, measure := range t.Tablature.Measures {
		if !fractionPattern.MatchString(measure.TimeSignature) {
			return fmt.Errorf("measure %d: invalid time signature %q", i+1, measure.TimeSignature)
		}
		for _, note := range measure.Notes {
			if len(note.Frets) != len(t.Instrument.Tuning) {
				return fmt.Errorf("measure %d: incorrect number of frets", i+1)
			}
			for _, fret := range note.Frets {
				if fret != nil && *fret < 0 {
					return fmt.Errorf("measure %d: negative fret number", i+1)
				}
			}
			if !fractionPattern.MatchString(note.Offset) {
				return fmt.Errorf("measure %d: invalid offset %q", i+1, note.Offset)
			}
			if note.Arpeggio < 0 || note.Vibration < 0 {
				return fmt.Errorf("measure %d: arpeggio and vibration must not be negative", i+1)
			}
		}
	}
	return nil
}

// BeatsPerMeasure is the top number of the time signature.
func (m Measure) BeatsPerMeasure() int {
	beats, _ := parseFraction(m.TimeSignature)
	return int(beats)
}

// NoteValue is the bottom number of the time signature: 4 means that one
// beat lasts a quarter note.
func (m Measure) NoteValue() int {
	_, value := parseFraction(m.TimeSignature)
	return int(value)
}

// parseFraction splits a validated "num/den" string into two numbers.
func parseFraction(s string) (num, den int64) {
	fmt.Sscanf(s, "%d/%d", &num, &den)
	return num, den
}

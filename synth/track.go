package synth

import "time"

// AudioTrack is a growing sequence of samples that sounds can be mixed into
// at any point in time.
type AudioTrack struct {
	SampleRate int
	Samples    []float64
}

// NewAudioTrack returns an empty track.
func NewAudioTrack(sampleRate int) *AudioTrack {
	return &AudioTrack{SampleRate: sampleRate}
}

// Duration returns the current length of the track.
func (t *AudioTrack) Duration() time.Duration {
	return Seconds(float64(len(t.Samples)) / float64(t.SampleRate))
}

// AddAt mixes samples into the track starting at the given instant. The track
// grows (padded with silence) when the new samples reach past its end.
func (t *AudioTrack) AddAt(instant time.Duration, samples []float64) {
	offset := NumSamples(instant, t.SampleRate)
	if end := offset + len(samples); end > len(t.Samples) {
		t.Samples = append(t.Samples, make([]float64, end-len(t.Samples))...)
	}
	for i, value := range samples {
		t.Samples[offset+i] += value
	}
}

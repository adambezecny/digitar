// Package audio reads and writes WAV files.
package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
)

// WriteWAV saves samples in [-1, 1] as a mono, 16-bit PCM WAV file.
// Values outside that range are clipped.
func WriteWAV(path string, samples []float64, sampleRate int) error {
	var buf bytes.Buffer
	le := binary.LittleEndian
	dataSize := uint32(len(samples) * 2)

	buf.WriteString("RIFF")
	binary.Write(&buf, le, 36+dataSize)
	buf.WriteString("WAVEfmt ")
	binary.Write(&buf, le, uint32(16))           // fmt chunk size
	binary.Write(&buf, le, uint16(1))            // PCM
	binary.Write(&buf, le, uint16(1))            // mono
	binary.Write(&buf, le, uint32(sampleRate))   // samples per second
	binary.Write(&buf, le, uint32(sampleRate*2)) // bytes per second
	binary.Write(&buf, le, uint16(2))            // bytes per sample frame
	binary.Write(&buf, le, uint16(16))           // bits per sample
	buf.WriteString("data")
	binary.Write(&buf, le, dataSize)

	for _, s := range samples {
		s = math.Max(-1, math.Min(1, s))
		binary.Write(&buf, le, int16(s*math.MaxInt16))
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// ReadWAV loads a PCM (8/16/24/32-bit) or 32-bit float WAV file and mixes all
// channels down to mono. It returns samples in [-1, 1] and the sample rate.
func ReadWAV(path string) ([]float64, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("%s: not a WAV file", path)
	}

	le := binary.LittleEndian
	var format, channels, bits uint16
	var sampleRate uint32
	var pcm []byte

	// A WAV file is a list of chunks: 4-byte ID, 4-byte size, then the payload.
	for pos := 12; pos+8 <= len(data); {
		id := string(data[pos : pos+4])
		size := int(le.Uint32(data[pos+4 : pos+8]))
		body := data[pos+8 : min(pos+8+size, len(data))]
		switch id {
		case "fmt ":
			format = le.Uint16(body[0:2])
			channels = le.Uint16(body[2:4])
			sampleRate = le.Uint32(body[4:8])
			bits = le.Uint16(body[14:16])
			if format == 0xFFFE && len(body) >= 26 { // WAVE_FORMAT_EXTENSIBLE
				format = le.Uint16(body[24:26])
			}
		case "data":
			pcm = body
		}
		pos += 8 + size + size%2 // chunks are padded to an even size
	}
	if channels == 0 || pcm == nil {
		return nil, 0, fmt.Errorf("%s: missing fmt or data chunk", path)
	}

	width := int(bits / 8)
	decode := func(b []byte) float64 {
		switch {
		case format == 3 && bits == 32:
			return float64(math.Float32frombits(le.Uint32(b)))
		case format == 1 && bits == 8:
			return (float64(b[0]) - 128) / 128
		case format == 1 && bits == 16:
			return float64(int16(le.Uint16(b))) / (1 << 15)
		case format == 1 && bits == 24:
			v := int32(b[0]) | int32(b[1])<<8 | int32(int8(b[2]))<<16
			return float64(v) / (1 << 23)
		case format == 1 && bits == 32:
			return float64(int32(le.Uint32(b))) / (1 << 31)
		}
		return math.NaN()
	}
	if math.IsNaN(decode(make([]byte, width))) {
		return nil, 0, errors.New(path + ": unsupported WAV sample format")
	}

	frameSize := width * int(channels)
	samples := make([]float64, len(pcm)/frameSize)
	for i := range samples {
		frame := pcm[i*frameSize:]
		var sum float64
		for c := range int(channels) {
			sum += decode(frame[c*width:])
		}
		samples[i] = sum / float64(channels)
	}
	return samples, int(sampleRate), nil
}

package main

import (
	"encoding/binary"
	"math"
	"math/rand"
	"os"
)

// sampleRate is how many audio samples are played per second.
// 44100 is the CD-quality standard: each second of sound is
// 44,100 numbers between -1.0 and 1.0.
const sampleRate = 44100

// KarplusStrong synthesizes the sound of a plucked string.
//
//	freq     – the pitch of the note in Hz (e.g. 440 = A4)
//	duration – how long the sound lasts, in seconds
//	decay    – how fast the sound fades (0.98 = short pluck,
//	           0.999 = long ringing note). Must be below 1.0,
//	           otherwise the sound would never die out.
//
// It returns a slice of samples in the range [-1, 1].
func KarplusStrong(freq, duration, decay float64) []float64 {
	// --- STEP 1: Decide the length of the "string" ---------------------
	//
	// The buffer is our virtual string. The sound repeats every n
	// samples, and that repetition is what we hear as pitch.
	//
	// If we want 440 repeats per second and we play 44100 samples
	// per second, each repeat must be 44100 / 440 ≈ 100 samples long.
	//
	// Longer buffer  -> fewer repeats per second -> lower note.
	// Shorter buffer -> more repeats per second  -> higher note.
	n := int(math.Round(sampleRate / freq))
	buf := make([]float64, n)

	// --- STEP 2: "Pluck" the string ------------------------------------
	//
	// When you pluck a real string, it starts out vibrating chaotically.
	// We imitate that by filling the buffer with random noise.
	// rand.Float64() gives [0, 1); *2 - 1 shifts it to [-1, 1).
	for i := range buf {
		buf[i] = rand.Float64()*2 - 1
	}

	// Total number of samples to produce, e.g. 0.6 s * 44100 = 26460.
	total := int(duration * sampleRate)
	out := make([]float64, total)

	// --- STEP 3: Let the string vibrate --------------------------------
	//
	// We walk around the buffer in a circle, over and over.
	// Each time we visit a slot, we:
	//   a) send its current value to the output (that's the sound), and
	//   b) replace it with a slightly smoother, slightly quieter value,
	//      so the next time around the loop it sounds a bit different.
	for i := 0; i < total; i++ {
		// i % n wraps the index around: 0,1,2,...,n-1,0,1,2,...
		// This is what makes the buffer behave like a loop.
		idx := i % n

		// The neighbour slot, also wrapping around at the end.
		next := (idx + 1) % n

		// a) Output the current value of the string at this point.
		out[i] = buf[idx]

		// b) Update the string:
		//
		//    0.5 * (buf[idx] + buf[next])
		//      Average this sample with its neighbour. Averaging smooths
		//      out sharp jumps, which removes high (harsh) frequencies.
		//      Each pass round the loop makes the noise a little smoother,
		//      so it quickly turns into a clean, mellow tone, just as a
		//      real string loses its bright "twang" first.
		//
		//    decay * ...
		//      Shrink the value a tiny bit every pass so the sound
		//      gradually fades to silence, like a real string losing
		//      energy to the air.
		buf[idx] = decay * 0.5 * (buf[idx] + buf[next])
	}

	return out
}

// writeWAV saves samples to a .wav file (mono, 16-bit PCM) so any
// audio player can play it. This part has nothing to do with
// Karplus-Strong itself; it is just the file format.
func writeWAV(path string, samples []float64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Each sample is stored as a 16-bit integer = 2 bytes.
	dataSize := uint32(len(samples) * 2)

	// WAV files store numbers in little-endian byte order.
	le := binary.LittleEndian

	// --- WAV header (44 bytes describing the audio format) ---
	f.Write([]byte("RIFF"))                   // file type marker
	binary.Write(f, le, 36+dataSize)          // remaining file size
	f.Write([]byte("WAVEfmt "))               // format section starts
	binary.Write(f, le, uint32(16))           // size of format section
	binary.Write(f, le, uint16(1))            // 1 = uncompressed PCM
	binary.Write(f, le, uint16(1))            // 1 channel (mono)
	binary.Write(f, le, uint32(sampleRate))   // samples per second
	binary.Write(f, le, uint32(sampleRate*2)) // bytes per second
	binary.Write(f, le, uint16(2))            // bytes per sample frame
	binary.Write(f, le, uint16(16))           // bits per sample
	f.Write([]byte("data"))                   // audio data starts
	binary.Write(f, le, dataSize)             // size of audio data

	// --- Audio data ---
	for _, s := range samples {
		// Clip to [-1, 1] so we never overflow the 16-bit range.
		s = math.Max(-1, math.Min(1, s))

		// Convert [-1.0, 1.0] to the 16-bit integer range [-32767, 32767].
		binary.Write(f, le, int16(s*32767))
	}
	return nil
}

func main() {
	// Frequencies of a C-major arpeggio: C4, E4, G4, C5.
	// C5 is exactly double C4 (one octave higher).
	notes := []float64{261.63, 329.63, 392.00, 523.25}

	// Generate each note and join them one after another.
	var song []float64
	for _, f := range notes {
		// 0.6 seconds per note, decay 0.996 = a nice guitar-like sustain.
		song = append(song, KarplusStrong(f, 0.6, 0.996)...)
	}

	// Save the result. Play it with: afplay pluck.wav
	if err := writeWAV("/tmp/pluck.wav", song); err != nil {
		panic(err)
	}
}

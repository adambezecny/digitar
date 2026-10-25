package effects

import (
	"math"
	"math/cmplx"
)

// convolve returns the first len(x) samples of the convolution of x and h.
// It multiplies their spectra (via the FFT) instead of sliding h over x,
// which would take hours for a song and a one-second impulse response.
func convolve(x, h []float64) []float64 {
	n := 1
	for n < len(x)+len(h)-1 {
		n <<= 1
	}
	a := make([]complex128, n)
	b := make([]complex128, n)
	for i, v := range x {
		a[i] = complex(v, 0)
	}
	for i, v := range h {
		b[i] = complex(v, 0)
	}

	fft(a, false)
	fft(b, false)
	for i := range a {
		a[i] *= b[i]
	}
	fft(a, true)

	out := make([]float64, len(x))
	for i := range out {
		out[i] = real(a[i])
	}
	return out
}

// fft is an in-place iterative radix-2 fast Fourier transform.
// len(a) must be a power of two. inverse=true computes the inverse transform.
func fft(a []complex128, inverse bool) {
	n := len(a)

	// Reorder the elements by bit-reversed index.
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}

	// Combine ever larger blocks ("butterflies").
	for size := 2; size <= n; size <<= 1 {
		angle := 2 * math.Pi / float64(size)
		if !inverse {
			angle = -angle
		}
		step := cmplx.Rect(1, angle)
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := range size / 2 {
				u := a[start+k]
				v := a[start+k+size/2] * w
				a[start+k] = u + v
				a[start+k+size/2] = u - v
				w *= step
			}
		}
	}

	if inverse {
		for i := range a {
			a[i] /= complex(float64(n), 0)
		}
	}
}

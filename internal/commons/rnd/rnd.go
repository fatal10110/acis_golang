// Package rnd is the server's central randomness provider — gameplay
// dice-rolling, not cryptographic or protocol-level randomness (those have
// their own dedicated code). Gameplay randomness has no fixed bit-for-bit
// sequence contract; callers rely on uniform distributions.
package rnd

import "math/rand/v2"

// Get returns a pseudo-random int in [0, n). It panics if n <= 0.
func Get(n int) int {
	return rand.IntN(n)
}

// GetRange returns a pseudo-random int in [min, max], inclusive of both
// ends. It panics if max < min.
func GetRange(min, max int) int {
	return min + rand.IntN(max-min+1)
}

// GetFloat returns a pseudo-random float64 in [0, n).
func GetFloat(n float64) float64 {
	return rand.Float64() * n
}

// NextGaussian returns a pseudo-random float64 from the standard normal
// distribution (mean 0, standard deviation 1).
func NextGaussian() float64 {
	return rand.NormFloat64()
}

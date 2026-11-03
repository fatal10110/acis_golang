package npcs

import "math"

// inPrivateRing reports whether d, a private's distance from its master,
// fits the spawn point drawn around the master: a random offset in
// [minOffset, maxOffset] at a random angle, whose X and Y parts are each
// truncated toward zero. The truncation shortens the distance by less than
// √2 and never lengthens it.
func inPrivateRing(d float64, minOffset, maxOffset int) bool {
	return d > float64(minOffset)-math.Sqrt2 && d <= float64(maxOffset)
}

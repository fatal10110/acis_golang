package player

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

func closeFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// testModOwner returns a fresh, disposable stat-Mod owner identity for
// tests that need one only to attach a Mod, never to exercise
// RemoveStatsByOwner's identity matching.
func testModOwner() effect.ModOwner {
	return effect.ModOwnerEffect(&effect.Effect{})
}

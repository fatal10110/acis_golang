package spawn

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
)

// CalculateRespawnDelay returns the next respawn delay for entry: its base
// delay randomized by up to ±RespawnRandom (clamped so the randomized
// spread never pushes the result below zero). It returns zero when the
// entry has no respawn delay configured, meaning it does not respawn.
func CalculateRespawnDelay(entry Entry) time.Duration {
	if entry.RespawnDelay <= 0 {
		return 0
	}

	random := entry.RespawnRandom
	if random > entry.RespawnDelay {
		random = entry.RespawnDelay
	}
	if random <= 0 {
		return entry.RespawnDelay
	}

	offset := rnd.GetRange(-int(random), int(random))
	return entry.RespawnDelay + time.Duration(offset)
}

package spawn

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
)

// CalculateRespawnDelay returns the next respawn delay for entry: its base
// delay plus a whole number of seconds drawn evenly from
// [-RespawnRandom, RespawnRandom], the random part clamped so the result
// never goes below zero. It returns zero when the entry has no respawn
// delay configured, meaning it does not respawn.
func CalculateRespawnDelay(entry Entry) time.Duration {
	if entry.RespawnDelay <= 0 {
		return 0
	}

	random := entry.RespawnRandom
	if random > entry.RespawnDelay {
		random = entry.RespawnDelay
	}
	seconds := int(random / time.Second)
	if seconds <= 0 {
		return entry.RespawnDelay
	}

	return entry.RespawnDelay + time.Duration(rnd.GetRange(-seconds, seconds))*time.Second
}

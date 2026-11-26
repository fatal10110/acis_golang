package spawn

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Position is one explicit spawn position from the "pos" attribute.
type Position struct {
	Location location.Location
	Heading  int
	Chance   int
}

// Private is one child NPC declared under a spawn entry's <privates> block.
type Private struct {
	NPCID        int32
	Weight       int
	RespawnDelay time.Duration
}

// Entry is one <npc .../> row under an <npcmaker>.
type Entry struct {
	NPCID         int32
	Total         int
	RespawnDelay  time.Duration
	RespawnRandom time.Duration
	Positions     []Position
	Privates      []Private
	AIParams      map[string]string
	// AIParamKeys are the names of AIParams in the order the entry first
	// declares each.
	AIParamKeys []string
	DBName      string
	DBSaving    []string
}

// NewEntry returns e as the spawnlist loads it: Total is scaled and
// rounded by multiplier (the configured spawn multiplier) only when
// Positions is empty, distinguishing a fixed/weighted "pos" list (total
// kept as declared) from a coordinate-less territory spawn (total scaled).
// Empty Positions, Privates, AIParams and DBSaving read as nil.
func NewEntry(e Entry, multiplier float64) Entry {
	if len(e.Positions) == 0 {
		e.Total = scaleBySpawnMultiplier(e.Total, multiplier)
	}
	e.Positions = append([]Position(nil), e.Positions...)
	e.Privates = append([]Private(nil), e.Privates...)
	e.AIParams = copyStringMap(e.AIParams)
	e.AIParamKeys = append([]string(nil), e.AIParamKeys...)
	e.DBSaving = append([]string(nil), e.DBSaving...)
	return e
}

func copyStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

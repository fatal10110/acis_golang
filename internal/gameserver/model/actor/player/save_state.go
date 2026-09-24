package player

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// SaveState is the character-row values one save writes, copied at one
// instant so the write can run later without reading the live character.
type SaveState struct {
	ID                int32
	Progression       Progression
	Resources         Resources
	Karma             int
	PvPKills          int
	PKKills           int
	DeathPenaltyLevel int
	// OnlineTime is the lifetime playtime in seconds at the copy.
	OnlineTime int64
	Location   location.Location
	Heading    int
}

// SaveState copies c's persisted character-row values.
func (c *Character) SaveState() SaveState {
	progression := c.ProgressionValues()
	return SaveState{
		ID:                c.ID,
		Progression:       progression,
		Resources:         c.ResourceValues(),
		Karma:             progression.Karma,
		PvPKills:          progression.PvPKills,
		PKKills:           progression.PKKills,
		DeathPenaltyLevel: c.DeathPenaltyLevel(),
		OnlineTime:        c.TotalOnlineTime(time.Now()), // persisted wall-clock total, not a queue deadline
		Location:          c.CurrentLocation(),
		Heading:           c.CurrentHeading(),
	}
}

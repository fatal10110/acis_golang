package player

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// SaveState is the character-row values one save writes, copied at one
// instant so the write can run later without reading the live character.
type SaveState struct {
	ID int32
	// ClassID is the class played at the copy, and BaseClassID the base
	// class.
	ClassID     int
	BaseClassID int
	// Progression is the base class's: the characters row always holds
	// it, whichever class is active.
	Progression Progression
	// Subclasses are the subclass slots, each with its own progression.
	Subclasses        []SubClass
	Resources         Resources
	Karma             int
	PvPKills          int
	PKKills           int
	DeathPenaltyLevel int
	// OnlineTime is the lifetime playtime in seconds at the copy.
	OnlineTime int64
	Location   location.Location
	Heading    int
	// WantsPeace is the personal-surrender flag (see WantsPeace).
	WantsPeace bool
	// Noble is the noblesse status (see IsNoble).
	Noble bool
	// In7sDungeon is the Seven Signs dungeon membership (see In7sDungeon).
	In7sDungeon bool
	// VarkaKetraAlliance is the faction standing (see VarkaKetraAlliance).
	VarkaKetraAlliance int
	// ClientDetached marks a copy taken while the character lingers in the
	// world after its connection dropped: the row's online column then
	// records 2 instead of 1 (Player.isOnlineInt, Player.java:4504-4510).
	// The character model does not know its session, so the caller sets it.
	ClientDetached bool
}

// SaveState copies c's persisted character-row values.
func (c *Character) SaveState() SaveState {
	c.progressionMu.RLock()
	progression := c.progressionLocked()
	progression.CharLevel, progression.Exp, progression.SP = c.baseProgressionLocked()
	subs := c.subclassesLocked()
	classID := c.ClassID()
	c.progressionMu.RUnlock()
	// A character aboard a boat is saved on the shore of the dock the boat
	// serves, not out at sea; an observer at the position it left to
	// watch.
	at := c.CurrentLocation()
	if v := c.Boat(); v != nil {
		at = v.OustLocation()
	} else if saved, ok := c.SavedLocation(); ok && c.ObserverMode() {
		at = saved
	}
	return SaveState{
		ID:                 c.ID,
		ClassID:            classID,
		BaseClassID:        c.BaseClassID(),
		Progression:        progression,
		Subclasses:         subs,
		Resources:          c.ResourceValues(),
		Karma:              progression.Karma,
		PvPKills:           progression.PvPKills,
		PKKills:            progression.PKKills,
		DeathPenaltyLevel:  c.DeathPenaltyLevel(),
		OnlineTime:         c.TotalOnlineTime(time.Now()), // persisted wall-clock total, not a queue deadline
		Location:           at,
		Heading:            c.CurrentHeading(),
		WantsPeace:         c.WantsPeace(),
		Noble:              c.IsNoble(),
		In7sDungeon:        c.In7sDungeon(),
		VarkaKetraAlliance: c.VarkaKetraAlliance(),
	}
}

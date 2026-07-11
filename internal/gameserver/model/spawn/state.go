package spawn

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Status is the persisted lifecycle state of one DB-backed spawn row.
type Status int16

const (
	// StatusUninitialized marks a DB-backed spawn declared by XML but not
	// spawned yet; such rows are not persisted.
	StatusUninitialized Status = -1
	// StatusDead marks a spawn waiting for its respawn time.
	StatusDead Status = 0
	// StatusAlive marks a live spawn whose HP, MP and location can be restored.
	StatusAlive Status = 1
)

// State is one spawn_data row. Callers that mutate a live spawn concurrently
// must guard the owning spawn aggregate; State itself is a plain value holder.
type State struct {
	Name        string
	Status      Status
	CurrentHP   int
	CurrentMP   int
	Location    location.Location
	Heading     int
	DBValue     int
	RespawnTime int64
}

// NewState returns an uninitialized dynamic spawn state for name.
func NewState(name string) *State {
	return &State{Name: name, Status: StatusUninitialized}
}

// Dead reports whether the state still represents a dead spawn waiting for
// its respawn deadline.
func (s *State) Dead(now time.Time) bool {
	return s.Status == StatusDead && s.RespawnTime > 0 && s.RespawnTime > now.UnixMilli()
}

// CheckAlive applies the startup restore rule. It returns true when the
// persisted alive data should be reused. It returns false after initializing
// a missing row or expiring a dead row into a fresh live spawn.
func (s *State) CheckAlive(loc location.Location, heading, maxHP, maxMP int, now time.Time) bool {
	if (s.Status == StatusDead && s.RespawnTime > 0 && s.RespawnTime <= now.UnixMilli()) || s.Status < 0 {
		s.Status = StatusAlive
		s.CurrentHP = maxHP
		s.CurrentMP = maxMP
		s.Location = loc
		s.Heading = heading
		s.RespawnTime = 0
		return false
	}
	return true
}

// SetStats stores the live HP, MP and position for this spawn. Dead rows keep
// their respawn deadline intact.
func (s *State) SetStats(hp, mp int, loc location.Location, heading int) {
	if s.Status == StatusDead {
		return
	}
	s.CurrentHP = hp
	s.CurrentMP = mp
	s.Location = loc
	s.Heading = heading
	s.RespawnTime = 0
}

// SetRespawn stores a dead state with a respawn deadline delay after now.
func (s *State) SetRespawn(delay time.Duration, now time.Time) {
	s.Status = StatusDead
	s.CurrentHP = 0
	s.CurrentMP = 0
	s.Location = location.Location{}
	s.Heading = 0
	s.RespawnTime = now.Add(delay).UnixMilli()
}

// CancelRespawn clears a scheduled respawn without turning the row into a
// fresh uninitialized row.
func (s *State) CancelRespawn() {
	s.RespawnTime = 1
}

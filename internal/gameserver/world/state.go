package world

import (
	"strings"
	"sync"
)

// State tracks every live object, player, and pet currently in the game
// world, alongside the spatial grid (embedded *Grid) used to index them by
// position.
type State struct {
	*Grid

	objects *registry[Tracked]
	players *registry[Player]
	summons *registry[Tracked] // keyed by the owner's id, not the summon's own id

	// playersMu serializes AddPlayer/RemovePlayer so the players registry
	// mutation and the playerNames index mutation happen as one atomic
	// step; two independently-locked steps let a same-id AddPlayer and
	// RemovePlayer interleave and leave a stale playerNames entry that
	// blocks a later, different player from registering under that name.
	playersMu   sync.RWMutex
	playerNames map[string]int32

	// mu is the world lock. It serializes every change of region membership
	// and region activity, and every placement except a Move that stays in
	// its region (see Move). It is held only for the in-memory update of one
	// placement — Discover/Forget and region-activity callbacks always run
	// after it is released. Region.mu is the only lock taken under it, and
	// only around a region's slice update; known-list scans take Region.mu
	// alone, never mu.
	mu sync.RWMutex
	// idle is signaled whenever a placement finishes delivering its
	// callbacks; a placement of a busy object waits on it (see
	// Presence.busy). Its Locker is mu's write side.
	idle sync.Cond
}

// New returns an empty State with a freshly built region grid.
func New() *State {
	s := &State{
		Grid:        NewGrid(),
		objects:     newRegistry[Tracked](),
		players:     newRegistry[Player](),
		summons:     newRegistry[Tracked](),
		playerNames: make(map[string]int32),
	}
	s.idle.L = &s.mu
	return s
}

// AddObject starts tracking obj, unless an object with the same id is
// already tracked.
func (s *State) AddObject(obj Tracked) { s.objects.add(obj.ObjectID(), obj) }

// RemoveObject stops tracking the object with the given id.
func (s *State) RemoveObject(id int32) { s.objects.remove(id) }

// removeObjectIfSame stops tracking obj only if it is still the object
// registered under its own id. See registry.removeIfSame.
func (s *State) removeObjectIfSame(obj Tracked) bool {
	return s.objects.removeIfSame(obj.ObjectID(), obj)
}

// RemoveObjects stops tracking every object with the given ids.
func (s *State) RemoveObjects(ids []int32) { s.objects.removeAll(ids) }

// Object returns the tracked object with the given id, if any.
func (s *State) Object(id int32) (Tracked, bool) { return s.objects.get(id) }

// Objects returns a snapshot of every tracked object.
func (s *State) Objects() []Tracked { return s.AppendObjects(nil) }

// AppendObjects appends every tracked object to dst and returns the
// extended slice, matching Region.appendObjects' append (not replace)
// contract — pass dst[:0] for a fresh scan. A caller that keeps dst across
// repeat calls (e.g. a per-tick scratch buffer owned by a single
// goroutine) pays the allocation only until the buffer's capacity
// stabilizes at the tracked population size.
func (s *State) AppendObjects(dst []Tracked) []Tracked {
	return s.objects.appendAll(dst)
}

// AddPlayer marks obj online, unless a player with the same id is already
// tracked, and indexes its name for PlayerByName lookups. The registry and
// name-index updates happen under one lock so a concurrent RemovePlayer for
// the same id can't interleave between them.
func (s *State) AddPlayer(obj Player) {
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	s.players.add(obj.ObjectID(), obj)

	name := strings.ToLower(obj.CharacterName())
	if _, exists := s.playerNames[name]; !exists {
		s.playerNames[name] = obj.ObjectID()
	}
}

// RemovePlayer marks the player with the given id offline and drops its
// name index entry, under the same lock as AddPlayer.
func (s *State) RemovePlayer(id int32) {
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	if obj, ok := s.players.get(id); ok {
		name := strings.ToLower(obj.CharacterName())
		if s.playerNames[name] == id {
			delete(s.playerNames, name)
		}
	}

	s.players.remove(id)
}

// Player returns the online player with the given id, if any.
func (s *State) Player(id int32) (Player, bool) { return s.players.get(id) }

// PlayerByName returns the online player with the given name, matched
// case-insensitively, mirroring Java's World.getPlayer(String).
func (s *State) PlayerByName(name string) (Player, bool) {
	s.playersMu.RLock()
	id, ok := s.playerNames[strings.ToLower(name)]
	s.playersMu.RUnlock()
	if !ok {
		return nil, false
	}
	return s.Player(id)
}

// Players returns a snapshot of every online player.
func (s *State) Players() []Player { return s.players.appendAll(nil) }

// AddSummon marks summon as ownerID's active pet or servitor, unless that
// owner already has one tracked.
func (s *State) AddSummon(ownerID int32, summon Tracked) { s.summons.add(ownerID, summon) }

// RemoveSummon clears ownerID's active pet or servitor, if any.
func (s *State) RemoveSummon(ownerID int32) { s.summons.remove(ownerID) }

// Summon returns ownerID's active pet or servitor, if any.
func (s *State) Summon(ownerID int32) (Tracked, bool) { return s.summons.get(ownerID) }

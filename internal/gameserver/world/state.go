package world

import (
	"strings"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
)

// State tracks every live object, player, and pet currently in the game
// world, alongside the spatial grid (embedded *Grid) used to index them by
// position.
type State struct {
	*Grid

	objects *registry
	players *registry
	pets    *registry // keyed by the pet owner's id, not the pet's own id

	// playersMu serializes AddPlayer/RemovePlayer so the players registry
	// mutation and the playerNames index mutation happen as one atomic
	// step; two independently-locked steps let a same-id AddPlayer and
	// RemovePlayer interleave and leave a stale playerNames entry that
	// blocks a later, different player from registering under that name.
	playersMu   sync.RWMutex
	playerNames map[string]int32

	// regionActivityMu serializes a Player's region-activity accounting
	// (relocate's playersCount updates plus the resulting Region.setActive
	// calls) against every other concurrent Player relocation, closing the
	// check-then-act race between one player's departure deactivating a
	// region and another's arrival activating it. Only relocate calls for a
	// Player take it; non-player traffic (the overwhelming majority —
	// NPCs, ground items, doors) never touches it.
	regionActivityMu sync.Mutex
}

// New returns an empty State with a freshly built region grid.
func New() *State {
	return &State{
		Grid:        NewGrid(),
		objects:     newRegistry(),
		players:     newRegistry(),
		pets:        newRegistry(),
		playerNames: make(map[string]int32),
	}
}

// namedPlayer is implemented by every player registered through AddPlayer;
// it backs the name-to-online-player-ID index without widening
// worldobject.Object for every kind of tracked object.
type namedPlayer interface {
	CharacterName() string
}

// AddObject starts tracking obj, unless an object with the same id is
// already tracked.
func (s *State) AddObject(obj worldobject.Object) { s.objects.add(obj.ObjectID(), obj) }

// RemoveObject stops tracking the object with the given id.
func (s *State) RemoveObject(id int32) { s.objects.remove(id) }

// removeObjectIfSame stops tracking obj only if it is still the object
// registered under its own id. See registry.removeIfSame.
func (s *State) removeObjectIfSame(obj worldobject.Object) bool {
	return s.objects.removeIfSame(obj.ObjectID(), obj)
}

// RemoveObjects stops tracking every object with the given ids.
func (s *State) RemoveObjects(ids []int32) { s.objects.removeAll(ids) }

// Object returns the tracked object with the given id, if any.
func (s *State) Object(id int32) (worldobject.Object, bool) { return s.objects.get(id) }

// Objects returns a snapshot of every tracked object.
func (s *State) Objects() []worldobject.Object { return s.objects.all() }

// AddPlayer marks obj online, unless a player with the same id is already
// tracked, and indexes its name for PlayerByName lookups. The registry and
// name-index updates happen under one lock so a concurrent RemovePlayer for
// the same id can't interleave between them.
func (s *State) AddPlayer(obj worldobject.Object) {
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	s.players.add(obj.ObjectID(), obj)

	if np, ok := obj.(namedPlayer); ok {
		name := strings.ToLower(np.CharacterName())
		if _, exists := s.playerNames[name]; !exists {
			s.playerNames[name] = obj.ObjectID()
		}
	}
}

// RemovePlayer marks the player with the given id offline and drops its
// name index entry, under the same lock as AddPlayer.
func (s *State) RemovePlayer(id int32) {
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	if obj, ok := s.players.get(id); ok {
		if np, ok := obj.(namedPlayer); ok {
			name := strings.ToLower(np.CharacterName())
			if s.playerNames[name] == id {
				delete(s.playerNames, name)
			}
		}
	}

	s.players.remove(id)
}

// Player returns the online player with the given id, if any.
func (s *State) Player(id int32) (worldobject.Object, bool) { return s.players.get(id) }

// PlayerByName returns the online player with the given name, matched
// case-insensitively, mirroring Java's World.getPlayer(String).
func (s *State) PlayerByName(name string) (worldobject.Object, bool) {
	s.playersMu.RLock()
	id, ok := s.playerNames[strings.ToLower(name)]
	s.playersMu.RUnlock()
	if !ok {
		return nil, false
	}
	return s.Player(id)
}

// Players returns a snapshot of every online player.
func (s *State) Players() []worldobject.Object { return s.players.all() }

// AddPet marks pet as ownerID's active pet, unless that owner already has
// one tracked.
func (s *State) AddPet(ownerID int32, pet worldobject.Object) { s.pets.add(ownerID, pet) }

// AddSummon marks summon as ownerID's active pet or servitor.
func (s *State) AddSummon(ownerID int32, summon worldobject.Object) { s.AddPet(ownerID, summon) }

// RemovePet clears ownerID's active pet, if any.
func (s *State) RemovePet(ownerID int32) { s.pets.remove(ownerID) }

// RemoveSummon clears ownerID's active pet or servitor, if any.
func (s *State) RemoveSummon(ownerID int32) { s.RemovePet(ownerID) }

// Pet returns ownerID's active pet, if any.
func (s *State) Pet(ownerID int32) (worldobject.Object, bool) { return s.pets.get(ownerID) }

// Summon returns ownerID's active pet or servitor, if any.
func (s *State) Summon(ownerID int32) (worldobject.Object, bool) { return s.Pet(ownerID) }

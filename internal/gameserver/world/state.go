package world

import "github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"

// State tracks every live object, player, and pet currently in the game
// world, alongside the spatial grid (embedded *Grid) used to index them by
// position.
type State struct {
	*Grid

	objects *registry
	players *registry
	pets    *registry // keyed by the pet owner's id, not the pet's own id
}

// New returns an empty State with a freshly built region grid.
func New() *State {
	return &State{
		Grid:    NewGrid(),
		objects: newRegistry(),
		players: newRegistry(),
		pets:    newRegistry(),
	}
}

// AddObject starts tracking obj, unless an object with the same id is
// already tracked.
func (s *State) AddObject(obj worldobject.Object) { s.objects.add(obj.ObjectID(), obj) }

// RemoveObject stops tracking the object with the given id.
func (s *State) RemoveObject(id int32) { s.objects.remove(id) }

// RemoveObjects stops tracking every object with the given ids.
func (s *State) RemoveObjects(ids []int32) { s.objects.removeAll(ids) }

// Object returns the tracked object with the given id, if any.
func (s *State) Object(id int32) (worldobject.Object, bool) { return s.objects.get(id) }

// Objects returns a snapshot of every tracked object.
func (s *State) Objects() []worldobject.Object { return s.objects.all() }

// AddPlayer marks obj online, unless a player with the same id is already
// tracked.
func (s *State) AddPlayer(obj worldobject.Object) { s.players.add(obj.ObjectID(), obj) }

// RemovePlayer marks the player with the given id offline.
func (s *State) RemovePlayer(id int32) { s.players.remove(id) }

// Player returns the online player with the given id, if any.
func (s *State) Player(id int32) (worldobject.Object, bool) { return s.players.get(id) }

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

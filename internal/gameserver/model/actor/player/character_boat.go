package player

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Vessel is a boat a character can ride.
type Vessel interface {
	ObjectID() int32
	// OustLocation is the shore point of the dock the boat serves now,
	// where a passenger who logs out aboard comes back.
	OustLocation() location.Location
}

// boatRide is a character's standing with boats: the boat it rides, where
// it stands on the deck, in the boat's own coordinates, and the two flags
// the boarding packets trade (a walk on or toward a boat in progress, and
// leave to board). mu guards every field: the owner's queue writes them,
// and other queues read them (a viewer building CharInfo, a boat's fare).
type boatRide struct {
	mu       sync.Mutex
	vessel   Vessel
	position location.Location
	heading  int
	movement bool
	canBoard bool
}

// Boat returns the boat c rides, or nil.
func (c *Character) Boat() Vessel {
	c.boat.mu.Lock()
	defer c.boat.mu.Unlock()
	return c.boat.vessel
}

// InBoat reports whether c rides a boat.
func (c *Character) InBoat() bool { return c.Boat() != nil }

// BoatObjectID returns the object id of the boat c rides, or 0.
func (c *Character) BoatObjectID() int32 {
	if v := c.Boat(); v != nil {
		return v.ObjectID()
	}
	return 0
}

// Board makes v the boat c rides. Leaving a boat (v nil) also clears c's
// position on its deck.
func (c *Character) Board(v Vessel) {
	c.boat.mu.Lock()
	defer c.boat.mu.Unlock()
	if v == nil && c.boat.vessel != nil {
		c.boat.position, c.boat.heading = location.Location{}, 0
	}
	c.boat.vessel = v
}

// BoatPosition returns where c stands on its boat's deck, in the boat's
// coordinates, and the heading it faces there.
func (c *Character) BoatPosition() (location.Location, int) {
	c.boat.mu.Lock()
	defer c.boat.mu.Unlock()
	return c.boat.position, c.boat.heading
}

// SetBoatPosition moves c on its boat's deck to at, keeping its heading.
func (c *Character) SetBoatPosition(at location.Location) {
	c.boat.mu.Lock()
	defer c.boat.mu.Unlock()
	c.boat.position = at
}

// SetBoatPositionHeading moves c on its boat's deck to at, facing heading.
func (c *Character) SetBoatPositionHeading(at location.Location, heading int) {
	c.boat.mu.Lock()
	defer c.boat.mu.Unlock()
	c.boat.position, c.boat.heading = at, heading
}

// BoatMovement reports a walk on or toward a boat in progress.
func (c *Character) BoatMovement() bool {
	c.boat.mu.Lock()
	defer c.boat.mu.Unlock()
	return c.boat.movement
}

// SetBoatMovement marks a walk on or toward a boat in progress, or over.
func (c *Character) SetBoatMovement(moving bool) {
	c.boat.mu.Lock()
	defer c.boat.mu.Unlock()
	c.boat.movement = moving
}

// CanBoard reports whether c has leave to board a boat.
func (c *Character) CanBoard() bool {
	c.boat.mu.Lock()
	defer c.boat.mu.Unlock()
	return c.boat.canBoard
}

// SetCanBoard grants or withdraws c's leave to board a boat.
func (c *Character) SetCanBoard(can bool) {
	c.boat.mu.Lock()
	defer c.boat.mu.Unlock()
	c.boat.canBoard = can
}

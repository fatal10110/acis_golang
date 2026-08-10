package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// X returns the current world X coordinate.
func (c *Character) X() int {
	return c.CurrentLocation().X
}

// Y returns the current world Y coordinate.
func (c *Character) Y() int {
	return c.CurrentLocation().Y
}

// Z returns the current world Z coordinate.
func (c *Character) Z() int {
	return c.CurrentLocation().Z
}

// SetFlightBroadcaster records the packet-layer hook for forced movement.
func (c *Character) SetFlightBroadcaster(broadcast func(location.Location, modelskill.Flight)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastFlight = broadcast
}

// SetPositionBroadcaster records the packet-layer hook for a forced-location
// correction after a flight lands.
func (c *Character) SetPositionBroadcaster(broadcast func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastPosition = broadcast
}

// FlyTo broadcasts a forced-flight animation without changing server position.
func (c *Character) FlyTo(dest location.Location, flight modelskill.Flight) {
	c.stateMu.RLock()
	broadcast := c.broadcastFlight
	c.stateMu.RUnlock()
	if broadcast != nil {
		broadcast(dest, flight)
	}
}

// SetXYZ moves the character immediately and reseeds its ordinary movement
// state so the next move starts from the forced landing.
func (c *Character) SetXYZ(x, y, z int) {
	position := location.Location{X: x, Y: y, Z: z}
	c.SyncPosition(position)
	if c.Live != nil {
		c.Move().SetPosition(position)
	}
}

// BroadcastPosition sends the forced-location correction after a flight lands.
func (c *Character) BroadcastPosition() {
	c.stateMu.RLock()
	broadcast := c.broadcastPosition
	c.stateMu.RUnlock()
	if broadcast != nil {
		broadcast()
	}
}

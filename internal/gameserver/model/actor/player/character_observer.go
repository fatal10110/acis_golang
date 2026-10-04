package player

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// savedLocation is the position a character left to watch from a
// viewpoint, and returns to afterwards. The character's own queue writes
// it; saves and the checks of other characters read it.
type savedLocation struct {
	mu  sync.Mutex
	at  location.Location
	set bool
}

// SavedLocation is the position the character left to observe, false when
// none is held.
func (c *Character) SavedLocation() (location.Location, bool) {
	c.saved.mu.Lock()
	defer c.saved.mu.Unlock()
	return c.saved.at, c.saved.set
}

// SetSavedLocation records the position the character leaves to observe.
func (c *Character) SetSavedLocation(at location.Location) {
	c.saved.mu.Lock()
	c.saved.at, c.saved.set = at, true
	c.saved.mu.Unlock()
}

// ClearSavedLocation drops the held position.
func (c *Character) ClearSavedLocation() {
	c.saved.mu.Lock()
	c.saved.at, c.saved.set = location.Location{}, false
	c.saved.mu.Unlock()
}

// ObserverMode reports whether the character is spectating: it holds a
// saved position without competing in an Olympiad match.
func (c *Character) ObserverMode() bool {
	if c.OlympiadMode() {
		return false
	}
	_, ok := c.SavedLocation()
	return ok
}

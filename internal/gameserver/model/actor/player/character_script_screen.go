package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ShowScreenMessage shows c text at the top center of its screen for
// durationMs milliseconds.
func (c *Character) ShowScreenMessage(text string, durationMs int32) {
	c.emit(event.ScreenMessageShown{Text: text, DurationMs: durationMs})
}

// ShakeScreen shakes c's screen around where c stands now, with intensity,
// for duration seconds; no one else feels it.
func (c *Character) ShakeScreen(intensity, duration int32) {
	x, y, z := c.Position()
	c.emit(event.EarthquakeStarted{At: location.Location{X: x, Y: y, Z: z}, Intensity: intensity, Duration: duration})
}

// ShowMultisell shows c the multisell list name as the civilian NPC
// objectID offers it.
func (c *Character) ShowMultisell(objectID int32, name string) {
	c.emit(event.MultisellRequested{ObjectID: objectID, List: name})
}

package player

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// TutorialEvents takes the tutorial events characters raise.
type TutorialEvents interface {
	// TutorialEvent hands the event to c's tutorial quest, when c has a
	// state in it; otherwise nothing happens.
	TutorialEvent(c *Character, e TutorialEvent)
}

// TutorialEvent is an event a character's tutorial quest hears, under the
// name the quest's event hook receives.
type TutorialEvent string

// TutorialEnterWorld: the character entered the world.
const TutorialEnterWorld TutorialEvent = "UC"

// TutorialClientEvent returns the tutorial event of client event id: "CE"
// and the id.
func TutorialClientEvent(id int32) TutorialEvent {
	return TutorialEvent("CE" + strconv.Itoa(int(id)))
}

// The client event ids of the tutorial events the character raises itself.
const (
	// tutorialDied: HP damage killed the character outside an Olympiad
	// match.
	tutorialDied = 30
	// tutorialLeveledUp: the character's level went up.
	tutorialLeveledUp = 40
	// tutorialLowHP: an HP write left the character below lowHPRatio of its
	// maximum HP.
	tutorialLowHP = 45
	// TutorialSitID: the character sat down. The tutorial also enables it
	// as a client event and keeps it in its journal while it waits for it.
	TutorialSitID = 8388608
)

// NotifyTutorial hands the tutorial event e to c's tutorial quest. It runs
// on the caller, which must hold none of c's locks.
func (c *Character) NotifyTutorial(e TutorialEvent) {
	if c.tutorial != nil {
		c.tutorial.TutorialEvent(c, e)
	}
}

// NotifyTutorialOnQueue is NotifyTutorial for a caller running a task of
// c's own queue: the tutorial's hook runs as a hook on that queue (see
// RunOnOwnQueue), so what it asks of c runs inside the call.
func (c *Character) NotifyTutorialOnQueue(e TutorialEvent) {
	c.RunOnOwnQueue(func() { c.NotifyTutorial(e) })
}

// ShowTutorialPage opens c's tutorial window on the datapack page file.
func (c *Character) ShowTutorialPage(file string) {
	c.emit(event.TutorialPageShown{File: file})
}

// CloseTutorialPage closes c's tutorial window.
func (c *Character) CloseTutorialPage() {
	c.emit(event.TutorialPageClosed{})
}

// ShowTutorialQuestionMark shows c the tutorial question mark id.
func (c *Character) ShowTutorialQuestionMark(id int32) {
	c.emit(event.TutorialQuestionMarkShown{ID: id})
}

// EnableTutorialClientEvent makes c's client report the tutorial client
// event id when it happens.
func (c *Character) EnableTutorialClientEvent(id int32) {
	c.emit(event.TutorialClientEventEnabled{ID: id})
}

// PlayTutorialVoice plays c the tutorial voice file, from where c stands.
func (c *Character) PlayTutorialVoice(voice string) {
	c.emit(event.TutorialVoicePlayed{Voice: voice})
}

// AddRadarMarker marks x, y, z on c's radar. No list of markers is kept:
// nothing reads one back.
func (c *Character) AddRadarMarker(x, y, z int32) {
	c.emit(event.RadarMarkerAdded{X: x, Y: y, Z: z})
}

// RemoveRadarMarker removes the mark at x, y, z from c's radar.
func (c *Character) RemoveRadarMarker(x, y, z int32) {
	c.emit(event.RadarMarkerRemoved{X: x, Y: y, Z: z})
}

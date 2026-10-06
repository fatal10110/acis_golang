package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/memo"
)

// TutorialEvents takes the tutorial events characters raise.
type TutorialEvents interface {
	// TutorialEvent hands the event name to c's tutorial quest, when c has
	// a state in it; otherwise nothing happens.
	TutorialEvent(c *Character, name string)
}

// The tutorial events the character raises itself.
const (
	// TutorialEnterWorld: the character entered the world.
	TutorialEnterWorld = "UC"
	// tutorialDied: HP damage killed the character outside the Olympiad.
	tutorialDied = "CE30"
	// tutorialLeveledUp: the character's level went up.
	tutorialLeveledUp = "CE40"
	// tutorialLowHP: an HP write left the character below lowHPRatio of its
	// maximum HP.
	tutorialLowHP = "CE45"
	// TutorialSat: the character sat down.
	TutorialSat = "CE8388608"
)

// lowHPRatio is the share of maximum HP below which an HP write raises
// tutorialLowHP.
const lowHPRatio = 0.3

// NotifyTutorial hands the tutorial event name to c's tutorial quest. It
// runs on the caller, which must hold none of c's locks.
func (c *Character) NotifyTutorial(name string) {
	if c.tutorial != nil {
		c.tutorial.TutorialEvent(c, name)
	}
}

// writeHPLocked is the one place c's current HP is written, except the HP
// clear of a character restored dead (MarkDead). It marks the write for
// lowHPNotice, which every writer reaches after it releases vitalsMu
// (SettleRegen runs it). The caller holds vitalsMu.
func (c *Character) writeHPLocked(hp float64) {
	c.curHP = hp
	c.hpWritten.Store(true)
}

// lowHPNotice raises tutorialLowHP once per marked HP write that left c
// below lowHPRatio of its maximum HP. It runs after vitalsMu is released,
// right after the writer's own status report when it sends one.
func (c *Character) lowHPNotice() {
	if !c.hpWritten.Swap(false) || c.tutorial == nil {
		return
	}
	if res := c.ResourceValues(); res.CurrentHP/res.MaxHP < lowHPRatio {
		c.NotifyTutorial(tutorialLowHP)
	}
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

// Memos returns c's memos.
func (c *Character) Memos() *memo.Memos {
	return &c.memos
}

// DieFromDamage runs Die for HP damage that emptied c's HP, then, outside
// the Olympiad, hands c's tutorial quest the death event. It reports
// whether this call killed c.
func (c *Character) DieFromDamage(killer attackable.Combatant) bool {
	if !c.Die(killer) {
		return false
	}
	if !c.OlympiadMode() {
		c.NotifyTutorial(tutorialDied)
	}
	return true
}

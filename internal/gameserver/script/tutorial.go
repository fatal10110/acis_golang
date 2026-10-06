package script

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
)

// tutorialQuest is the quest the tutorial events go to.
const tutorialQuest = "Tutorial"

// tutorialPages is the datapack directory of the tutorial window's pages.
const tutorialPages = "data/html/script/feature/Tutorial/"

// TutorialEvent runs the event hook of c's tutorial quest with the event
// name and no NPC, when c has a state in that quest; otherwise nothing
// happens. The tutorial client requests and the game's tutorial triggers
// raise it, on the goroutine they run on. What the hook answers is shown to
// c with no NPC.
func (r *Registry) TutorialEvent(c *player.Character, name string) {
	st := c.Quests().State(tutorialQuest)
	if st == nil {
		return
	}
	s := r.byName[strings.ToLower(st.Quest().Name)]
	if s == nil {
		return
	}
	res := r.answer(s, hookEvent, func() string { return s.Hooks.Event(s, Event{Name: name, Player: PlayerOf(c)}) })
	s.show(c, nil, res)
}

var _ player.TutorialEvents = (*Registry)(nil)

// CharacterCreated gives c, a character just created and never in the
// world, a started tutorial quest state when the tutorial is registered,
// so its first enter world reaches the tutorial. The state's row is
// written on c's persistence lane; c's journal is then sealed, so its
// first selection waits for that write and is refused if it never lands.
func (r *Registry) CharacterCreated(c *player.Character) {
	s := r.byName[strings.ToLower(tutorialQuest)]
	if s == nil || s.env == nil || s.env.Quests == nil {
		return
	}
	q := s.env.Quests
	q.NewState(c, s).SetStatus(questlog.StatusStarted)
	q.Seal(c)
}

// characterOrNil returns the player the handle is on, nil for a handle on
// nothing.
func (p *Player) characterOrNil() *player.Character {
	if h, ok := p.combatant().(player.CharacterHolder); ok {
		return h.PlayerCharacter()
	}
	return nil
}

// The tutorial and radar helpers below do nothing on a handle on nothing.

// ShowTutorialHTML opens the player's tutorial window on the tutorial page
// file.
func (p *Player) ShowTutorialHTML(file string) {
	if c := p.characterOrNil(); c != nil {
		c.ShowTutorialPage(tutorialPages + file)
	}
}

// CloseTutorialHTML closes the player's tutorial window.
func (p *Player) CloseTutorialHTML() {
	if c := p.characterOrNil(); c != nil {
		c.CloseTutorialPage()
	}
}

// ShowQuestionMark shows the player the tutorial question mark id.
func (p *Player) ShowQuestionMark(id int32) {
	if c := p.characterOrNil(); c != nil {
		c.ShowTutorialQuestionMark(id)
	}
}

// EnableTutorialEvent makes the player's client report the tutorial client
// event id when it happens.
func (p *Player) EnableTutorialEvent(id int32) {
	if c := p.characterOrNil(); c != nil {
		c.EnableTutorialClientEvent(id)
	}
}

// PlayTutorialVoice plays the player the tutorial voice file.
func (p *Player) PlayTutorialVoice(voice string) {
	if c := p.characterOrNil(); c != nil {
		c.PlayTutorialVoice(voice)
	}
}

// AddRadarMarker marks x, y, z on the player's radar.
func (p *Player) AddRadarMarker(x, y, z int32) {
	if c := p.characterOrNil(); c != nil {
		c.AddRadarMarker(x, y, z)
	}
}

// RemoveRadarMarker removes the mark at x, y, z from the player's radar.
func (p *Player) RemoveRadarMarker(x, y, z int32) {
	if c := p.characterOrNil(); c != nil {
		c.RemoveRadarMarker(x, y, z)
	}
}

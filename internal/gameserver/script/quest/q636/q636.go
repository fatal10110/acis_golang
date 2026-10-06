// Package q636 is the quest The Truth Beyond the Gate: Priest Eliyah sends
// the player to Priest Flauron for a Visitor's Mark, which fades as its
// holder steps into the Pagan Temple's gate.
package q636

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q636_TruthBeyondTheGate"

// NPCs.
const (
	eliyah  = 31329
	flauron = 32010
)

// Rewards.
const (
	visitorMark      = 8064
	fadedVisitorMark = 8065
)

// templeGate is the zone at the Pagan Temple's gate.
const templeGate = 100000

// minLevel is the level the quest starts at.
const minLevel = 73

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "The Truth Beyond the Gate",
		QuestID: 636,
		Bind: script.Bindings{
			script.EventQuestStart: {eliyah},
			script.EventTalked:     {eliyah, flauron},
		},
		EnteredZones: []int32{templeGate},
		Hooks:        script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnZoneEnter: onZoneEnter},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31329-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "32010-02.htm"):
		s.GiveItems(e.Player, visitorMark, 1)
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(false)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < minLevel {
			htmltext = "31329-01.htm"
		} else {
			htmltext = "31329-02.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case eliyah:
			htmltext = "31329-05.htm"
		case flauron:
			if e.Player.HasItem(visitorMark) {
				htmltext = "32010-03.htm"
			} else {
				htmltext = "32010-01.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// onZoneEnter fades the Visitor's Mark of any player stepping into the
// gate, whatever its quest state: the quest has usually ended by then.
func onZoneEnter(s *script.Script, e script.ZoneEnter) {
	p, ok := e.Creature.(*script.Player)
	if !ok {
		return
	}
	if s.DestroyItems(p, visitorMark, 1) {
		s.AddItems(p, fadedVisitorMark, 1)
	}
}

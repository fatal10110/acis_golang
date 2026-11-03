// Package q113 is the quest Status of the Beacon Tower: Moira sends a box
// to Torrant, who pays adena for it.
package q113

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q113_StatusOfTheBeaconTower"

// NPCs.
const (
	moira   = 31979
	torrant = 32016
)

// box is the box Moira sends.
const box = 8086

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Status of the Beacon Tower",
		QuestID: 113,
		Items:   []int32{box},
		Bind: script.Bindings{
			script.EventQuestStart: {moira},
			script.EventTalked:     {moira, torrant},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "31979-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, box, 1)
	case strings.EqualFold(e.Name, "32016-02.htm"):
		s.TakeItems(p, box, 1)
		s.RewardItems(p, 57, 21578)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 40 {
			htmltext = "31979-00.htm"
		} else {
			htmltext = "31979-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case moira:
			htmltext = "31979-03.htm"
		case torrant:
			htmltext = "32016-01.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

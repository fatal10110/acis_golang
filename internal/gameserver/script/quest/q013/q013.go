// Package q013 is the quest Parcel Delivery: Fundin gives a player a
// package, which Vulcan takes for adena.
package q013

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q013_ParcelDelivery"

// NPCs.
const (
	fundin = 31274
	vulcan = 31539
)

// pkg is the quest item, the package.
const pkg = 7263

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Parcel Delivery",
		QuestID: 13,
		Items:   []int32{pkg},
		Bind: script.Bindings{
			script.EventQuestStart: {fundin},
			script.EventTalked:     {fundin, vulcan},
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
	switch {
	case strings.EqualFold(e.Name, "31274-2.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, pkg, 1)
	case strings.EqualFold(e.Name, "31539-1.htm"):
		s.TakeItems(e.Player, pkg, 1)
		s.RewardItems(e.Player, 57, 82656)
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
		if e.Player.Level() < 74 {
			htmltext = "31274-1.htm"
		} else {
			htmltext = "31274-0.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case fundin:
			htmltext = "31274-2.htm"
		case vulcan:
			htmltext = "31539-0.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

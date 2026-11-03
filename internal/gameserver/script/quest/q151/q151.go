// Package q151 is the quest Cure for Fever Disease: Elias sends the player
// after a poison sac, which Yohanes turns into the fever medicine Elias
// rewards.
package q151

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q151_CureForFeverDisease"

// Items.
const (
	poisonSac     = 703
	feverMedicine = 704
)

// NPCs.
const (
	elias   = 30050
	yohanes = 30032
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Cure for Fever Disease",
		QuestID: 151,
		Items:   []int32{feverMedicine, poisonSac},
		Bind: script.Bindings{
			script.EventQuestStart: {elias},
			script.EventTalked:     {elias, yohanes},
			script.EventMyDying:    {20103, 20106, 20108},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	if strings.EqualFold(e.Name, "30050-03.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 15 {
			htmltext = "30050-01.htm"
		} else {
			htmltext = "30050-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case elias:
			switch cond {
			case 1:
				htmltext = "30050-04.htm"
			case 2:
				htmltext = "30050-05.htm"
			case 3:
				htmltext = "30050-06.htm"
				s.TakeItems(p, feverMedicine, 1)
				s.GiveItems(p, 102, 1)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case yohanes:
			switch cond {
			case 2:
				htmltext = "30032-01.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, poisonSac, 1)
				s.GiveItems(p, feverMedicine, 1)
			case 3:
				htmltext = "30032-02.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	if s.DropItems(p, poisonSac, 1, 1, 200000) {
		st.SetCond(2)
	}
}

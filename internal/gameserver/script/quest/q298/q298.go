// Package q298 is the quest Lizardmen's Conspiracy: Praga sends the patrol
// report to Rohmer, who pays in SP for fifty white and fifty red gems of the
// lizardmen.
package q298

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q298_LizardmensConspiracy"

// NPCs.
const (
	praga  = 30333
	rohmer = 30344
)

// Items.
const (
	patrolReport = 7182
	whiteGem     = 7183
	redGem       = 7184
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Lizardmen's Conspiracy",
		QuestID: 298,
		Items:   []int32{patrolReport, whiteGem, redGem},
		Bind: script.Bindings{
			script.EventQuestStart: {praga},
			script.EventTalked:     {praga, rohmer},
			script.EventMyDying:    {20926, 20927, 20922, 20923, 20924},
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
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30333-1.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, patrolReport, 1)
	case strings.EqualFold(e.Name, "30344-1.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, patrolReport, 1)
	case strings.EqualFold(e.Name, "30344-4.htm"):
		if st.Cond() == 3 {
			htmltext = "30344-3.htm"
			s.TakeItems(p, whiteGem, -1)
			s.TakeItems(p, redGem, -1)
			s.RewardExpAndSp(p, 0, 42000)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 25 {
			htmltext = "30333-0b.htm"
		} else {
			htmltext = "30333-0a.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case praga:
			htmltext = "30333-2.htm"
		case rohmer:
			switch {
			case st.Cond() != 1:
				htmltext = "30344-2.htm"
			case p.HasItem(patrolReport):
				htmltext = "30344-0.htm"
			default:
				htmltext = "30344-0a.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMember(script.ActingPlayer(e.Killer), e.NPC, script.ByCond(2))
	if st == nil {
		return
	}
	p := st.Player()
	switch e.NPC.NpcID() {
	case 20922:
		if s.DropItems(p, whiteGem, 1, 50, 400000) && p.ItemCount(redGem) >= 50 {
			st.SetCond(3)
		}
	case 20923:
		if s.DropItems(p, whiteGem, 1, 50, 450000) && p.ItemCount(redGem) >= 50 {
			st.SetCond(3)
		}
	case 20924:
		if s.DropItems(p, whiteGem, 1, 50, 350000) && p.ItemCount(redGem) >= 50 {
			st.SetCond(3)
		}
	case 20926, 20927:
		if s.DropItems(p, redGem, 1, 50, 400000) && p.ItemCount(whiteGem) >= 50 {
			st.SetCond(3)
		}
	}
}

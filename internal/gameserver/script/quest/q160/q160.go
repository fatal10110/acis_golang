// Package q160 is the quest Nerupa's Request: an elf carries Nerupa's
// silvery spidersilk to Unoren, Creamees and Julia in turn and brings the
// nightshade leaf back.
package q160

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q160_NerupasRequest"

// Items.
const (
	silverySpidersilk = 1026
	unorenReceipt     = 1027
	creameesTicket    = 1028
	nightshadeLeaf    = 1029
)

// Reward.
const lesserHealingPotion = 1060

// NPCs.
const (
	nerupa   = 30370
	unoren   = 30147
	creamees = 30149
	julia    = 30152
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Nerupa's Request",
		QuestID: 160,
		Items:   []int32{silverySpidersilk, unorenReceipt, creameesTicket, nightshadeLeaf},
		Bind: script.Bindings{
			script.EventQuestStart: {nerupa},
			script.EventTalked:     {nerupa, unoren, creamees, julia},
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
	if strings.EqualFold(e.Name, "30370-04.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, silverySpidersilk, 1)
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
		switch {
		case p.Race() != player.RaceElf:
			htmltext = "30370-00.htm"
		case p.Level() < 3:
			htmltext = "30370-02.htm"
		default:
			htmltext = "30370-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case nerupa:
			if cond < 4 {
				htmltext = "30370-05.htm"
			} else if cond == 4 {
				htmltext = "30370-06.htm"
				s.TakeItems(p, nightshadeLeaf, 1)
				s.RewardItems(p, lesserHealingPotion, 5)
				s.RewardExpAndSp(p, 1000, 0)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case unoren:
			switch cond {
			case 1:
				htmltext = "30147-01.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, silverySpidersilk, 1)
				s.GiveItems(p, unorenReceipt, 1)
			case 2:
				htmltext = "30147-02.htm"
			case 4:
				htmltext = "30147-03.htm"
			}
		case creamees:
			switch cond {
			case 2:
				htmltext = "30149-01.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, unorenReceipt, 1)
				s.GiveItems(p, creameesTicket, 1)
			case 3:
				htmltext = "30149-02.htm"
			case 4:
				htmltext = "30149-03.htm"
			}
		case julia:
			switch cond {
			case 3:
				htmltext = "30152-01.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, creameesTicket, 1)
				s.GiveItems(p, nightshadeLeaf, 1)
			case 4:
				htmltext = "30152-02.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// Package q352 is the quest Help Rood Raise a New Pet: Rood buys the Lienrik
// eggs the character takes from the Lienriks.
package q352

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q352_HelpRoodRaiseANewPet"

// rood is the quest giver.
const rood = 31067

// Items.
const (
	lienrikEgg1 = 5860
	lienrikEgg2 = 5861
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Help Rood Raise A New Pet!",
		QuestID: 352,
		Items:   []int32{lienrikEgg1, lienrikEgg2},
		Bind: script.Bindings{
			script.EventQuestStart: {rood},
			script.EventTalked:     {rood},
			script.EventMyDying:    {20786, 20787, 21644, 21645},
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
	switch {
	case strings.EqualFold(e.Name, "31067-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31067-09.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
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
		if p.Level() < 39 {
			htmltext = "31067-00.htm"
		} else {
			htmltext = "31067-01.htm"
		}
	case questlog.StatusStarted:
		eggs1 := p.ItemCount(lienrikEgg1)
		eggs2 := p.ItemCount(lienrikEgg2)
		if eggs1+eggs2 == 0 {
			htmltext = "31067-05.htm"
			break
		}
		reward := int32(2000)
		switch {
		case eggs1 > 0 && eggs2 == 0:
			htmltext = "31067-06.htm"
			reward += eggs1 * 34
			s.TakeItems(p, lienrikEgg1, -1)
			s.RewardItems(p, 57, reward)
		case eggs1 == 0 && eggs2 > 0:
			htmltext = "31067-08.htm"
			reward += eggs2 * 1025
			s.TakeItems(p, lienrikEgg2, -1)
			s.RewardItems(p, 57, reward)
		case eggs1 > 0 && eggs2 > 0:
			htmltext = "31067-08.htm"
			reward += (eggs1 * 34) + (eggs2 * 1025) + 2000
			s.TakeItems(p, lienrikEgg1, -1)
			s.TakeItems(p, lienrikEgg2, -1)
			s.RewardItems(p, 57, reward)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	npcID := e.NPC.NpcID()
	random := s.Rnd(100)
	chance := 58
	if npcID == 20786 || npcID == 21644 {
		chance = 44
	}
	if random < chance {
		s.DropItemsAlways(p, lienrikEgg1, 1, 0)
	} else if random < (chance + 4) {
		s.DropItemsAlways(p, lienrikEgg2, 1, 0)
	}
}

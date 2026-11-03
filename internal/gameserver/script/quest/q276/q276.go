// Package q276 is the quest Totem of the Hestui: Tanapi gives an orc the
// Hestui totem and leather pants for the crystal of the Kasha bear totem
// spirit, which Kasha bears call once their parasites have spread.
package q276

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q276_TotemOfTheHestui"

// Items.
const (
	kashaParasite = 1480
	kashaCrystal  = 1481
)

// Rewards.
const (
	hestuiTotem  = 1500
	leatherPants = 29
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Totem of the Hestui",
		QuestID: 276,
		Items:   []int32{kashaParasite, kashaCrystal},
		Bind: script.Bindings{
			script.EventQuestStart: {30571}, // Tanapi
			script.EventTalked:     {30571},
			script.EventMyDying:    {20479, 27044},
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
	if strings.EqualFold(e.Name, "30571-03.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
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
		case p.Race() != player.RaceOrc:
			htmltext = "30571-00.htm"
		case p.Level() < 15:
			htmltext = "30571-01.htm"
		default:
			htmltext = "30571-02.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30571-04.htm"
		} else {
			htmltext = "30571-05.htm"
			s.TakeItems(p, kashaCrystal, -1)
			s.TakeItems(p, kashaParasite, -1)
			s.GiveItems(p, hestuiTotem, 1)
			s.GiveItems(p, leatherPants, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	if p.HasItem(kashaCrystal) {
		return
	}
	switch e.NPC.NpcID() {
	case 20479:
		count := p.ItemCount(kashaParasite)
		random := s.Rnd(100)
		if count >= 79 || (count >= 69 && random <= 20) || (count >= 59 && random <= 15) || (count >= 49 && random <= 10) || (count >= 39 && random < 2) {
			// The totem spirit stands where the bear fell.
			s.Spawner().AddSpawn(27044, e.NPC, true, 0)
			s.TakeItems(p, kashaParasite, count)
		} else {
			s.DropItemsAlways(p, kashaParasite, 1, 0)
		}
	case 27044:
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, kashaCrystal, 1)
	}
}

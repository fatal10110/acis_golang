// Package q293 is the quest The Hidden Veins: Filaur pays a dwarf for the
// chrysolite ores of the Utuku orcs and for the hidden vein maps Chinchirin
// pieces together from their torn fragments.
package q293

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q293_TheHiddenVeins"

// Items.
const (
	chrysoliteOre   = 1488
	tornMapFragment = 1489
	hiddenVeinMap   = 1490
)

// NPCs.
const (
	filaur     = 30535
	chinchirin = 30539
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "The Hidden Veins",
		QuestID: 293,
		Items:   []int32{chrysoliteOre, tornMapFragment, hiddenVeinMap},
		Bind: script.Bindings{
			script.EventQuestStart: {filaur},
			script.EventTalked:     {filaur, chinchirin},
			script.EventMyDying:    {20446, 20447, 20448}, // Utuku Orc, Utuku Archer, Utuku Grunt
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
	case strings.EqualFold(e.Name, "30535-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30535-06.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30539-02.htm"):
		if p.ItemCount(tornMapFragment) >= 4 {
			htmltext = "30539-03.htm"
			s.PlaySound(p, script.SoundItemGet)
			s.TakeItems(p, tornMapFragment, 4)
			s.GiveItems(p, hiddenVeinMap, 1)
		}
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
		switch {
		case p.Race() != player.RaceDwarf:
			htmltext = "30535-00.htm"
		case p.Level() < 6:
			htmltext = "30535-01.htm"
		default:
			htmltext = "30535-02.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case filaur:
			ores := p.ItemCount(chrysoliteOre)
			maps := p.ItemCount(hiddenVeinMap)
			if ores+maps == 0 {
				htmltext = "30535-04.htm"
				break
			}
			switch {
			case maps > 0 && ores > 0:
				htmltext = "30535-09.htm"
			case maps > 0:
				htmltext = "30535-08.htm"
			default:
				htmltext = "30535-05.htm"
			}
			s.TakeItems(p, chrysoliteOre, -1)
			s.TakeItems(p, hiddenVeinMap, -1)
			reward := ores*5 + maps*500
			if ores >= 10 {
				reward += 2000
			}
			s.RewardItems(p, 57, reward)
			s.RewardNewbieShots(p, 6000, 0)
		case chinchirin:
			htmltext = "30539-01.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	chance := s.Rnd(100)
	if chance > 50 {
		s.DropItemsAlways(p, chrysoliteOre, 1, 0)
	} else if chance < 5 {
		s.DropItemsAlways(p, tornMapFragment, 1, 0)
	}
}

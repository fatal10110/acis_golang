// Package q275 is the quest Dark Winged Spies: Tantus pays an orc 4200
// adena for seventy Darkwing bat fangs. A bat may call a Varangka's tracker,
// worth five fangs once killed.
package q275

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q275_DarkWingedSpies"

// Monsters.
const (
	darkwingBat     = 20316
	varangkaTracker = 27043
)

// Items.
const (
	darkwingBatFang  = 1478
	varangkaParasite = 1479
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Dark Winged Spies",
		QuestID: 275,
		Items:   []int32{darkwingBatFang, varangkaParasite},
		Bind: script.Bindings{
			script.EventQuestStart: {30567}, // Tantus
			script.EventTalked:     {30567},
			script.EventMyDying:    {darkwingBat, varangkaTracker},
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
	if strings.EqualFold(e.Name, "30567-03.htm") {
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
			htmltext = "30567-00.htm"
		case p.Level() < 11:
			htmltext = "30567-01.htm"
		default:
			htmltext = "30567-02.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30567-04.htm"
		} else {
			htmltext = "30567-05.htm"
			s.TakeItems(p, darkwingBatFang, -1)
			s.TakeItems(p, varangkaParasite, -1)
			s.RewardItems(p, 57, 4200)
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
	switch e.NPC.NpcID() {
	case darkwingBat:
		if s.DropItemsAlways(p, darkwingBatFang, 1, 70) {
			st.SetCond(2)
		} else if s.Rnd(100) < 10 && p.ItemCount(darkwingBatFang) > 10 && p.ItemCount(darkwingBatFang) < 66 {
			// The tracker stands where the bat fell.
			s.Spawner().AddSpawn(varangkaTracker, e.NPC, true, 0)
			s.GiveItems(p, varangkaParasite, 1)
		}
	case varangkaTracker:
		if p.HasItem(varangkaParasite) {
			s.TakeItems(p, varangkaParasite, -1)
			if s.DropItemsAlways(p, darkwingBatFang, 5, 70) {
				st.SetCond(2)
			}
		}
	}
}

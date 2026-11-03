// Package q294 is the quest Covert Business: Keef rewards a dwarf for a
// hundred bat fangs with a ring of raccoon, or adena once the ring is held.
package q294

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q294_CovertBusiness"

// Items.
const (
	batFang       = 1491
	ringOfRaccoon = 1508
)

const bardedBat = 20370

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Covert Business",
		QuestID: 294,
		Items:   []int32{batFang},
		Bind: script.Bindings{
			script.EventQuestStart: {30534}, // Keef
			script.EventTalked:     {30534},
			script.EventMyDying:    {bardedBat, 20480}, // Barded Bat, Blade Bat
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
	if strings.EqualFold(e.Name, "30534-03.htm") {
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
		switch {
		case p.Race() != player.RaceDwarf:
			htmltext = "30534-00.htm"
		case p.Level() < 10:
			htmltext = "30534-01.htm"
		default:
			htmltext = "30534-02.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30534-04.htm"
			break
		}
		if !p.HasItem(ringOfRaccoon) {
			htmltext = "30534-05.htm"
			s.TakeItems(p, batFang, -1)
			s.GiveItems(p, ringOfRaccoon, 1)
		} else {
			htmltext = "30534-06.htm"
			s.TakeItems(p, batFang, -1)
			s.RewardItems(p, 57, 2400)
		}
		s.RewardExpAndSp(p, 0, 600)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	var count int32 = 1
	chance := s.Rnd(10)
	isBarded := e.NPC.NpcID() == bardedBat
	threshold := 6
	if isBarded {
		threshold = 5
	}
	switch {
	case chance < 3:
		count++
	case chance < threshold:
		count += 2
	case isBarded && chance < 7:
		count += 3
	}
	if s.DropItemsAlways(p, batFang, count, 100) {
		st.SetCond(2)
	}
}

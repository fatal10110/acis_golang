// Package q295 is the quest Dreaming of the Skies: Arin rewards fifty
// floating stones of the magical weavers with a ring of firefly, or adena
// once the ring is held.
package q295

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q295_DreamingOfTheSkies"

// Items.
const (
	floatingStone = 1492
	ringOfFirefly = 1509
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Dreaming of the Skies",
		QuestID: 295,
		Items:   []int32{floatingStone},
		Bind: script.Bindings{
			script.EventQuestStart: {30536}, // Arin
			script.EventTalked:     {30536},
			script.EventMyDying:    {20153}, // Magical Weaver
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
	if strings.EqualFold(e.Name, "30536-03.htm") {
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
		if p.Level() < 11 {
			htmltext = "30536-01.htm"
		} else {
			htmltext = "30536-02.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "30536-04.htm"
			break
		}
		s.TakeItems(p, floatingStone, -1)
		if !p.HasItem(ringOfFirefly) {
			htmltext = "30536-05.htm"
			s.GiveItems(p, ringOfFirefly, 1)
		} else {
			htmltext = "30536-06.htm"
			s.RewardItems(p, 57, 2400)
		}
		s.RewardExpAndSp(p, 0, 500)
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
	var count int32 = 2
	if s.Rnd(100) > 25 {
		count = 1
	}
	if s.DropItemsAlways(p, floatingStone, count, 50) {
		st.SetCond(2)
	}
}

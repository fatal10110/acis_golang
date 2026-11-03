// Package q351 is the quest Black Swan: Captain Gosta sends the character
// after the Tasaba lizardmen, whose fangs and barrels Iason Heine buys, and
// Roman trades his bills for rewards.
package q351

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q351_BlackSwan"

// NPCs.
const (
	gosta      = 30916
	iasonHeine = 30969
	roman      = 30897
)

// Items.
const (
	orderOfGosta     = 4296
	lizardFang       = 4297
	barrelOfLeague   = 4298
	billOfIasonHeine = 4310
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Black Swan",
		QuestID: 351,
		Items:   []int32{orderOfGosta, barrelOfLeague, lizardFang},
		Bind: script.Bindings{
			script.EventQuestStart: {gosta},
			script.EventTalked:     {gosta, iasonHeine, roman},
			script.EventMyDying:    {20784, 20785, 21639, 21640},
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
	case strings.EqualFold(e.Name, "30916-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, orderOfGosta, 1)
	case strings.EqualFold(e.Name, "30969-02a.htm"):
		lizardFangs := p.ItemCount(lizardFang)
		if lizardFangs > 0 {
			htmltext = "30969-02.htm"
			s.TakeItems(p, lizardFang, -1)
			s.RewardItems(p, 57, lizardFangs*20+bonus(lizardFangs))
		}
	case strings.EqualFold(e.Name, "30969-03a.htm"):
		barrels := p.ItemCount(barrelOfLeague)
		if barrels > 0 {
			htmltext = "30969-03.htm"
			s.TakeItems(p, barrelOfLeague, -1)
			s.RewardItems(p, billOfIasonHeine, barrels)
			s.RewardItems(p, 57, barrels*20+bonus(barrels))
			// Heine sends the character to Roman, who exchanges the bills.
			if st.Cond() == 1 {
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
			}
		}
	case strings.EqualFold(e.Name, "30969-06.htm"):
		// Without both quest items the quest ends; otherwise a return page.
		if !p.HasItems(barrelOfLeague, lizardFang) {
			htmltext = "30969-07.htm"
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	}
	return htmltext
}

// bonus is the extra adena a sale of ten items or more earns.
func bonus(count int32) int32 {
	if count >= 10 {
		return 3880
	}
	return 0
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
		if p.Level() < 32 {
			htmltext = "30916-00.htm"
		} else {
			htmltext = "30916-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case gosta:
			htmltext = "30916-04.htm"
		case iasonHeine:
			htmltext = "30969-01.htm"
		case roman:
			if p.HasItem(billOfIasonHeine) {
				htmltext = "30897-01.htm"
			} else {
				htmltext = "30897-02.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	random := s.Rnd(4)
	if random < 3 {
		fangs := int32(2)
		if random < 2 {
			fangs = 1
		}
		s.DropItemsAlways(p, lizardFang, fangs, 0)
		s.DropItems(p, barrelOfLeague, 1, 0, 50000)
	} else {
		chance := int32(40000)
		if e.NPC.NpcID() > 20785 {
			chance = 30000
		}
		s.DropItems(p, barrelOfLeague, 1, 0, chance)
	}
}

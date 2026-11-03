// Package q360 is the quest Plunder Their Supplies: Coleman pays for the
// supply items taken from the Taik orcs, and five suspicious documents make
// a recipe of supply worth more.
package q360

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q360_PlunderTheirSupplies"

// Items.
const (
	supplyItem         = 5872
	suspiciousDocument = 5871
	recipeOfSupply     = 5870
)

// Monsters.
const (
	taikOrcSeeker       = 20666
	taikOrcSupplyLeader = 20669
)

// coleman is the quest giver.
const coleman = 30873

// droplist returns the drops of the monster npcID: the Taik Orc Seeker's,
// else the Supply Leader's.
func droplist(npcID int32) []script.DropInfo {
	if npcID == taikOrcSeeker {
		return []script.DropInfo{
			{ItemID: suspiciousDocument, Count: 1, Needed: 0, Chance: 50000},
			{ItemID: supplyItem, Count: 1, Needed: 0, Chance: 500000},
		}
	}
	return []script.DropInfo{
		{ItemID: suspiciousDocument, Count: 1, Needed: 0, Chance: 50000},
		{ItemID: supplyItem, Count: 1, Needed: 0, Chance: 660000},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Plunder Their Supplies",
		QuestID: 360,
		Items:   []int32{recipeOfSupply, supplyItem, suspiciousDocument},
		Bind: script.Bindings{
			script.EventQuestStart: {coleman},
			script.EventTalked:     {coleman},
			script.EventMyDying:    {taikOrcSeeker, taikOrcSupplyLeader},
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
	case strings.EqualFold(e.Name, "30873-2.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30873-6.htm"):
		s.TakeItems(e.Player, supplyItem, -1)
		s.TakeItems(e.Player, suspiciousDocument, -1)
		s.TakeItems(e.Player, recipeOfSupply, -1)
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
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 52 {
			htmltext = "30873-0a.htm"
		} else {
			htmltext = "30873-0.htm"
		}
	case questlog.StatusStarted:
		supplyItems := e.Player.ItemCount(supplyItem)
		if supplyItems == 0 {
			htmltext = "30873-3.htm"
		} else {
			reward := 6000 + supplyItems*100 + e.Player.ItemCount(recipeOfSupply)*6000
			htmltext = "30873-5.htm"
			s.TakeItems(e.Player, supplyItem, -1)
			s.TakeItems(e.Player, recipeOfSupply, -1)
			s.RewardItems(e.Player, 57, reward)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	s.DropMultipleItems(p, droplist(e.NPC.NpcID()), script.DropDivmod)
	if p.ItemCount(suspiciousDocument) == 5 {
		s.TakeItems(p, suspiciousDocument, 5)
		s.GiveItems(p, recipeOfSupply, 1)
	}
}

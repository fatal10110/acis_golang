// Package q355 is the quest Family Honor: Galibredo buys the busts the Timak
// orc troops carry, and Patrin appraises the works of Berona.
package q355

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q355_FamilyHonor"

// NPCs.
const (
	galibredo = 30181
	patrin    = 30929
)

// Monsters.
const (
	timakOrcTroopLeader  = 20767
	timakOrcTroopShaman  = 20768
	timakOrcTroopWarrior = 20769
	timakOrcTroopArcher  = 20770
)

// Items.
const (
	galibredoBust   = 4252
	workOfBerona    = 4350
	statuePrototype = 4351
	statueOriginal  = 4352
	statueReplica   = 4353
	statueForgery   = 4354
)

// chances returns each monster's drop chances out of 100, by NPC id: below
// the first a bust, below the second a work of Berona.
func chances() map[int32][]int {
	m := make(map[int32][]int, 4)
	m[timakOrcTroopLeader] = []int{44, 54}
	m[timakOrcTroopShaman] = []int{36, 45}
	m[timakOrcTroopWarrior] = []int{35, 43}
	m[timakOrcTroopArcher] = []int{32, 42}
	return m
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Family Honor",
		QuestID: 355,
		Items:   []int32{galibredoBust},
		Bind: script.Bindings{
			script.EventQuestStart: {galibredo},
			script.EventTalked:     {galibredo, patrin},
			script.EventMyDying:    {timakOrcTroopLeader, timakOrcTroopShaman, timakOrcTroopWarrior, timakOrcTroopArcher},
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
	case strings.EqualFold(e.Name, "30181-2.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30181-4b.htm"):
		count := p.ItemCount(galibredoBust)
		if count > 0 {
			htmltext = "30181-4.htm"
			reward := 2800 + (count * 120)
			if count >= 100 {
				htmltext = "30181-4a.htm"
				reward += 5000
			}
			s.TakeItems(p, galibredoBust, count)
			s.RewardItems(p, 57, reward)
		}
	case strings.EqualFold(e.Name, "30929-7.htm"):
		if p.HasItem(workOfBerona) {
			s.TakeItems(p, workOfBerona, 1)
			appraising := s.Rnd(100)
			switch {
			case appraising < 20:
				htmltext = "30929-2.htm"
			case appraising < 40:
				htmltext = "30929-3.htm"
				s.GiveItems(p, statueReplica, 1)
			case appraising < 60:
				htmltext = "30929-4.htm"
				s.GiveItems(p, statueOriginal, 1)
			case appraising < 80:
				htmltext = "30929-5.htm"
				s.GiveItems(p, statueForgery, 1)
			default:
				htmltext = "30929-6.htm"
				s.GiveItems(p, statuePrototype, 1)
			}
		}
	case strings.EqualFold(e.Name, "30181-6.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
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
		if p.Level() < 36 {
			htmltext = "30181-0a.htm"
		} else {
			htmltext = "30181-0.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case galibredo:
			if p.HasItem(galibredoBust) {
				htmltext = "30181-3a.htm"
			} else {
				htmltext = "30181-3.htm"
			}
		case patrin:
			htmltext = "30929-0.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	c := chances()[e.NPC.NpcID()]
	random := s.Rnd(100)
	if random < c[1] {
		item := int32(workOfBerona)
		if random < c[0] {
			item = galibredoBust
		}
		s.DropItemsAlways(st.Player(), item, 1, 0)
	}
}

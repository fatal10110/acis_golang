// Package q354 is the quest Conquest of Alligator Island: Kluck buys the
// alligator teeth of the island's lads, and ten torn map fragments make a
// pirate treasure map.
package q354

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q354_ConquestOfAlligatorIsland"

// kluck is the quest giver.
const kluck = 30895

// Items.
const (
	alligatorTooth    = 5863
	tornMapFragment   = 5864
	pirateTreasureMap = 5915
)

// droplist returns the drops of the monster npcID, nil for another.
func droplist(npcID int32) []script.DropInfo {
	var tooth int32
	switch npcID {
	case 20804: // Crokian Lad
		tooth = 490000
	case 20805: // Dailaon Lad
		tooth = 560000
	case 20806: // Crokian Lad Warrior
		tooth = 500000
	case 20807: // Farhite Lad
		tooth = 600000
	case 20808: // Nos Lad
		tooth = 690000
	case 20991: // Swamp Tribe
		tooth = 600000
	default:
		return nil
	}
	return []script.DropInfo{
		{ItemID: alligatorTooth, Count: 1, Needed: 0, Chance: tooth},
		{ItemID: tornMapFragment, Count: 1, Needed: 0, Chance: 100000},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Conquest of Alligator Island",
		QuestID: 354,
		Items:   []int32{alligatorTooth, tornMapFragment},
		Bind: script.Bindings{
			script.EventQuestStart: {kluck},
			script.EventTalked:     {kluck},
			script.EventMyDying:    {20804, 20805, 20806, 20807, 20808, 20991},
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
	case strings.EqualFold(e.Name, "30895-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30895-03.htm"):
		if p.HasItem(tornMapFragment) {
			htmltext = "30895-03a.htm"
		}
	case strings.EqualFold(e.Name, "30895-05.htm"):
		amount := p.ItemCount(alligatorTooth)
		if amount > 0 {
			reward := amount*220 + 3100
			if amount >= 100 {
				reward += 7600
				htmltext = "30895-05b.htm"
			} else {
				htmltext = "30895-05a.htm"
			}
			s.TakeItems(p, alligatorTooth, -1)
			s.RewardItems(p, 57, reward)
		}
	case strings.EqualFold(e.Name, "30895-07.htm"):
		if p.ItemCount(tornMapFragment) >= 10 {
			htmltext = "30895-08.htm"
			s.TakeItems(p, tornMapFragment, 10)
			s.GiveItems(p, pirateTreasureMap, 1)
			s.PlaySound(p, script.SoundItemGet)
		}
	case strings.EqualFold(e.Name, "30895-09.htm"):
		s.PlaySound(p, script.SoundFinish)
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
		if p.Level() < 38 {
			htmltext = "30895-00.htm"
		} else {
			htmltext = "30895-01.htm"
		}
	case questlog.StatusStarted:
		if p.HasItem(tornMapFragment) {
			htmltext = "30895-03a.htm"
		} else {
			htmltext = "30895-03.htm"
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
	s.DropMultipleItems(st.Player(), droplist(e.NPC.NpcID()), script.DropDivmod)
}

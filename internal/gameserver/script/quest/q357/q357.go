// Package q357 is the quest Warehouse Keeper's Ambition: Silva buys the jade
// crystals of the forest runners and the elders.
package q357

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q357_WarehouseKeepersAmbition"

// silva is the quest giver.
const silva = 30686

// jadeCrystal is the quest item.
const jadeCrystal = 5867

// Monsters.
const (
	forestRunner      = 20594
	flineElder        = 20595
	lieleElder        = 20596
	valleyTreantElder = 20597
)

// chance returns the monster npcID's crystal drop chance.
func chance(npcID int32) int32 {
	switch npcID {
	case forestRunner:
		return 400000
	case flineElder:
		return 410000
	case lieleElder:
		return 440000
	default:
		return 650000
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Warehouse Keeper's Ambition",
		QuestID: 357,
		Items:   []int32{jadeCrystal},
		Bind: script.Bindings{
			script.EventQuestStart: {silva},
			script.EventTalked:     {silva},
			script.EventMyDying:    {forestRunner, flineElder, lieleElder, valleyTreantElder},
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
	case strings.EqualFold(e.Name, "30686-2.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30686-7.htm"):
		count := p.ItemCount(jadeCrystal)
		if count == 0 {
			htmltext = "30686-4.htm"
		} else {
			reward := (count * 425) + 3500
			if count >= 100 {
				reward += 7400
			}
			s.TakeItems(p, jadeCrystal, -1)
			s.RewardItems(p, 57, reward)
		}
	case strings.EqualFold(e.Name, "30686-8.htm"):
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
		if p.Level() < 47 {
			htmltext = "30686-0a.htm"
		} else {
			htmltext = "30686-0.htm"
		}
	case questlog.StatusStarted:
		if !p.HasItem(jadeCrystal) {
			htmltext = "30686-4.htm"
		} else {
			htmltext = "30686-6.htm"
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
	s.DropItems(st.Player(), jadeCrystal, 1, 0, chance(e.NPC.NpcID()))
}

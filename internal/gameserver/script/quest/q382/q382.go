// Package q382 is the quest Kail's Magic Coin: Vergara lets the holder of a
// royal membership collect the magic coins of the Fallen Orcs.
package q382

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q382_KailsMagicCoin"

// Npcs.
const vergara = 30687

// Monsters.
const (
	fallenOrc        = 21017
	fallenOrcArcher  = 21019
	fallenOrcShaman  = 21020
	fallenOrcCaptain = 21022
)

// Items.
const (
	royalMembership = 5898
	silverBasilisk  = 5961
	goldGolem       = 5962
	bloodDragon     = 5963
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Kail's Magic Coin",
		QuestID: 382,
		Items:   []int32{silverBasilisk, goldGolem, bloodDragon},
		Bind: script.Bindings{
			script.EventQuestStart: {vergara},
			script.EventTalked:     {vergara},
			script.EventMyDying:    {fallenOrc, fallenOrcArcher, fallenOrcShaman, fallenOrcCaptain},
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
	if strings.EqualFold(e.Name, "30687-03.htm") {
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
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 55 || !e.Player.HasItem(royalMembership) {
			htmltext = "30687-01.htm"
		} else {
			htmltext = "30687-02.htm"
		}
	case questlog.StatusStarted:
		htmltext = "30687-04.htm"
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerCondition(p, e.NPC, 1) == nil {
		return
	}
	switch e.NPC.NpcID() {
	case fallenOrc:
		s.DropItems(p, silverBasilisk, 1, 0, 100000)
	case fallenOrcArcher:
		s.DropItems(p, goldGolem, 1, 0, 100000)
	case fallenOrcShaman:
		s.DropItems(p, bloodDragon, 1, 0, 100000)
	case fallenOrcCaptain:
		s.DropItems(p, 5961+int32(s.Rnd(3)), 1, 0, 100000)
	}
}

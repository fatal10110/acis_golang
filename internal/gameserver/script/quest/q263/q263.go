// Package q263 is the quest Orc Subjugation: Kayleen pays a dark elf for the
// amulets and necklaces of the orcs she hunts.
package q263

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q263_OrcSubjugation"

// Items.
const (
	orcAmulet   = 1116
	orcNecklace = 1117
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Orc Subjugation",
		QuestID: 263,
		Items:   []int32{orcAmulet, orcNecklace},
		Bind: script.Bindings{
			script.EventQuestStart: {30346}, // Kayleen
			script.EventTalked:     {30346},
			script.EventMyDying:    {20385, 20386, 20387, 20388},
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
	case strings.EqualFold(e.Name, "30346-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30346-06.htm"):
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
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case p.Race() != player.RaceDarkElf:
			htmltext = "30346-00.htm"
		case p.Level() < 8:
			htmltext = "30346-01.htm"
		default:
			htmltext = "30346-02.htm"
		}
	case questlog.StatusStarted:
		amulet := p.ItemCount(orcAmulet)
		necklace := p.ItemCount(orcNecklace)
		if amulet == 0 && necklace == 0 {
			htmltext = "30346-04.htm"
		} else {
			htmltext = "30346-05.htm"
			s.TakeItems(p, orcAmulet, -1)
			s.TakeItems(p, orcNecklace, -1)
			s.RewardItems(p, 57, amulet*20+necklace*30)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	itemID := int32(orcNecklace)
	if e.NPC.NpcID() == 20385 {
		itemID = orcAmulet
	}
	s.DropItems(p, itemID, 1, 0, 500000)
}

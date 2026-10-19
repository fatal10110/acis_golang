// Package q257 is the quest The Guard Is Busy: Gilbert pays a human for the
// amulets, necklaces and fangs of the orcs and werewolves around Gludio,
// with beginner shots on the first payment.
package q257

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q257_TheGuardIsBusy"

// Items.
const (
	gludioLordMark = 1084
	orcAmulet      = 752
	orcNecklace    = 1085
	werewolfFang   = 1086
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "The Guard Is Busy",
		QuestID: 257,
		Items:   []int32{orcAmulet, orcNecklace, werewolfFang, gludioLordMark},
		Bind: script.Bindings{
			script.EventQuestStart: {30039}, // Gilbert
			script.EventTalked:     {30039},
			script.EventMyDying:    {20006, 20093, 20096, 20098, 20130, 20131, 20132, 20342, 20343},
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
	case strings.EqualFold(e.Name, "30039-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, gludioLordMark, 1)
	case strings.EqualFold(e.Name, "30039-05.htm"):
		s.TakeItems(e.Player, gludioLordMark, 1)
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
		case p.Race() != player.RaceHuman:
			htmltext = "30039-00.htm"
		case p.Level() < 6:
			htmltext = "30039-01.htm"
		default:
			htmltext = "30039-02.htm"
		}
	case questlog.StatusStarted:
		amulets := p.ItemCount(orcAmulet)
		necklaces := p.ItemCount(orcNecklace)
		fangs := p.ItemCount(werewolfFang)
		if amulets+necklaces+fangs == 0 {
			htmltext = "30039-04.htm"
		} else {
			htmltext = "30039-07.htm"
			s.TakeItems(p, orcAmulet, -1)
			s.TakeItems(p, orcNecklace, -1)
			s.TakeItems(p, werewolfFang, -1)
			reward := 10*amulets + 20*(necklaces+fangs)
			if amulets+necklaces+fangs >= 10 {
				reward += 1000
			}
			s.RewardItems(p, 57, reward)
			s.RewardNewbieShots(p, 6000, 3000)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	switch e.NPC.NpcID() {
	case 20006, 20130, 20131:
		s.DropItems(p, orcAmulet, 1, 0, 500000)
	case 20093, 20096, 20098:
		s.DropItems(p, orcNecklace, 1, 0, 500000)
	case 20342:
		s.DropItems(p, werewolfFang, 1, 0, 200000)
	case 20343:
		s.DropItems(p, werewolfFang, 1, 0, 400000)
	case 20132:
		s.DropItems(p, werewolfFang, 1, 0, 500000)
	}
}

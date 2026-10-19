// Package q159 is the quest Protect the Water Source: Asterios sends an elf
// after the plague dust of the Plague Zombies, twice, for a hyacinth charm
// and adena.
package q159

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q159_ProtectTheWaterSource"

// Items.
const (
	plagueDust     = 1035
	hyacinthCharm1 = 1071
	hyacinthCharm2 = 1072
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Protect the Water Source",
		QuestID: 159,
		Items:   []int32{plagueDust, hyacinthCharm1, hyacinthCharm2},
		Bind: script.Bindings{
			script.EventQuestStart: {30154}, // Asterios
			script.EventTalked:     {30154},
			script.EventMyDying:    {27017}, // Plague Zombie
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
	if strings.EqualFold(e.Name, "30154-04.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, hyacinthCharm1, 1)
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
		switch {
		case p.Race() != player.RaceElf:
			htmltext = "30154-00.htm"
		case p.Level() < 12:
			htmltext = "30154-02.htm"
		default:
			htmltext = "30154-03.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "30154-05.htm"
		case 2:
			htmltext = "30154-06.htm"
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, plagueDust, -1)
			s.TakeItems(p, hyacinthCharm1, 1)
			s.GiveItems(p, hyacinthCharm2, 1)
		case 3:
			htmltext = "30154-07.htm"
		case 4:
			htmltext = "30154-08.htm"
			s.TakeItems(p, hyacinthCharm2, 1)
			s.TakeItems(p, plagueDust, -1)
			s.RewardItems(p, 57, 18250)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	if st.Cond() == 1 && s.DropItems(p, plagueDust, 1, 1, 400000) {
		st.SetCond(2)
	} else if st.Cond() == 3 && s.DropItems(p, plagueDust, 1, 5, 400000) {
		st.SetCond(4)
	}
}

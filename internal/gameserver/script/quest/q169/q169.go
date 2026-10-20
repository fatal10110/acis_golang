// Package q169 is the quest Offspring of Nightmares: Vlasty pays for the
// perfect skull of a dark horror, and a little more for each cracked one.
package q169

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q169_OffspringOfNightmares"

// Items.
const (
	crackedSkull = 1030
	perfectSkull = 1031
	boneGaiters  = 31
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Offspring of Nightmares",
		QuestID: 169,
		Items:   []int32{crackedSkull, perfectSkull},
		Bind: script.Bindings{
			script.EventQuestStart: {30145},
			script.EventTalked:     {30145},
			script.EventMyDying:    {20105, 20025},
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
	if strings.EqualFold(e.Name, "30145-04.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	} else if strings.EqualFold(e.Name, "30145-08.htm") {
		reward := 17000 + p.ItemCount(crackedSkull)*20
		s.TakeItems(p, perfectSkull, -1)
		s.TakeItems(p, crackedSkull, -1)
		s.GiveItems(p, boneGaiters, 1)
		s.RewardItems(p, 57, reward)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
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
			htmltext = "30145-00.htm"
		case p.Level() < 15:
			htmltext = "30145-02.htm"
		default:
			htmltext = "30145-03.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			if p.HasItem(crackedSkull) {
				htmltext = "30145-06.htm"
			} else {
				htmltext = "30145-05.htm"
			}
		case 2:
			htmltext = "30145-07.htm"
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
	if st.Cond() == 1 && s.DropItems(p, perfectSkull, 1, 1, 200000) {
		st.SetCond(2)
	} else {
		s.DropItems(p, crackedSkull, 1, 0, 500000)
	}
}

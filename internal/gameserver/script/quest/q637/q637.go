// Package q637 is the quest Through the Gate Once More: Flauron, for a
// character of level 73 or more who holds a faded visitor mark, asks for ten
// necromancer hearts and pays a pagan mark with them.
package q637

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q637_ThroughTheGateOnceMore"

// NPC.
const flauron = 32010

// Items.
const (
	fadedVisitorMark = 8065
	necromancerHeart = 8066
)

// Reward.
const paganMark = 8067

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Through the Gate Once More",
		QuestID: 637,
		Items:   []int32{necromancerHeart},
		Bind: script.Bindings{
			script.EventQuestStart: {flauron},
			script.EventTalked:     {flauron},
			script.EventMyDying:    {21565, 21566, 21567},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or ends it.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "32010-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "32010-10.htm"):
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
		switch {
		case e.Player.Level() < 73 || !e.Player.HasItem(fadedVisitorMark):
			htmltext = "32010-01a.htm"
		case e.Player.HasItem(paganMark):
			htmltext = "32010-00.htm"
		default:
			htmltext = "32010-01.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 2 {
			if e.Player.ItemCount(necromancerHeart) == 10 {
				htmltext = "32010-06.htm"
				s.TakeItems(e.Player, fadedVisitorMark, 1)
				s.TakeItems(e.Player, necromancerHeart, -1)
				s.GiveItems(e.Player, paganMark, 1)
				s.GiveItems(e.Player, 8273, 10)
				s.PlaySound(e.Player, script.SoundFinish)
				st.Exit(true)
			} else {
				st.SetCond(1)
			}
		} else {
			htmltext = "32010-05.htm"
		}
	}
	return htmltext
}

// onMyDying drops a necromancer heart to a party member at cond 1, and moves
// the one who then holds ten to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), necromancerHeart, 1, 10, 400000) {
		st.SetCond(2)
	}
}

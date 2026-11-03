// Package q643 is the quest Rise and Fall of the Elroki Tribe: Singsing, for
// a character of level 75 or more, pays adena for the bones the Elroki drop,
// and Karakawei trades three hundred bones for a scroll.
package q643

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q643_RiseAndFallOfTheElrokiTribe"

// NPCs.
const (
	singsing  = 32106
	karakawei = 32117
)

// Items.
const bones = 8776

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Rise and Fall of the Elroki Tribe",
		QuestID: 643,
		Items:   []int32{bones},
		Bind: script.Bindings{
			script.EventQuestStart: {singsing},
			script.EventTalked:     {singsing, karakawei},
			script.EventMyDying:    {22208, 22209, 22210, 22211, 22212, 22213, 22221, 22222, 22226, 22227},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, pays adena for the bones, trades three hundred
// of them for a scroll, or ends the quest.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "32106-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "32106-07.htm"):
		count := e.Player.ItemCount(bones)
		s.TakeItems(e.Player, bones, count)
		s.RewardItems(e.Player, 57, count*1374)
	case strings.EqualFold(e.Name, "32106-09.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "32117-03.htm"):
		count := e.Player.ItemCount(bones)
		if count >= 300 {
			s.TakeItems(e.Player, bones, 300)
			// A scroll of 8712 to 8722, both ends included.
			s.RewardItems(e.Player, 8712+int32(s.Rnd(8722-8712+1)), 5)
		} else {
			htmltext = "32117-04.htm"
		}
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 75 {
			htmltext = "32106-00.htm"
		} else {
			htmltext = "32106-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case singsing:
			if e.Player.HasItem(bones) {
				htmltext = "32106-06.htm"
			} else {
				htmltext = "32106-05.htm"
			}
		case karakawei:
			htmltext = "32117-01.htm"
		}
	}
	return htmltext
}

// onMyDying drops a bone to a party member who has the quest.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(st.Player(), bones, 1, 0, 750000)
}

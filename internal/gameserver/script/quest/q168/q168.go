// Package q168 is the quest Deliver Supplies: Jenna's letter goes to Harant,
// whose three sentry blades go to Jenna, Roselyn and Kristin, and the two
// old bronze swords come back to Jenna.
package q168

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q168_DeliverSupplies"

// Items.
const (
	jennaLetter    = 1153
	sentryBlade1   = 1154
	sentryBlade2   = 1155
	sentryBlade3   = 1156
	oldBronzeSword = 1157
)

// NPCs.
const (
	jenna   = 30349
	roselyn = 30355
	kristin = 30357
	harant  = 30360
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Deliver Supplies",
		QuestID: 168,
		Items:   []int32{jennaLetter, sentryBlade1, sentryBlade2, sentryBlade3, oldBronzeSword},
		Bind: script.Bindings{
			script.EventQuestStart: {jenna},
			script.EventTalked:     {jenna, roselyn, kristin, harant},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	if strings.EqualFold(e.Name, "30349-03.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, jennaLetter, 1)
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
			htmltext = "30349-00.htm"
		case p.Level() < 3:
			htmltext = "30349-01.htm"
		default:
			htmltext = "30349-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case jenna:
			switch cond {
			case 1:
				htmltext = "30349-04.htm"
			case 2:
				htmltext = "30349-05.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, sentryBlade1, 1)
			case 3:
				htmltext = "30349-07.htm"
			case 4:
				htmltext = "30349-06.htm"
				s.TakeItems(p, oldBronzeSword, 2)
				s.RewardItems(p, 57, 820)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case harant:
			if cond == 1 {
				htmltext = "30360-01.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, jennaLetter, 1)
				s.GiveItems(p, sentryBlade1, 1)
				s.GiveItems(p, sentryBlade2, 1)
				s.GiveItems(p, sentryBlade3, 1)
			} else if cond == 2 {
				htmltext = "30360-02.htm"
			}
		case roselyn:
			if cond == 3 {
				if p.HasItem(sentryBlade2) {
					htmltext = "30355-01.htm"
					s.TakeItems(p, sentryBlade2, 1)
					s.GiveItems(p, oldBronzeSword, 1)
					if p.ItemCount(oldBronzeSword) == 2 {
						st.SetCond(4)
						s.PlaySound(p, script.SoundMiddle)
					}
				} else {
					htmltext = "30355-02.htm"
				}
			} else if cond == 4 {
				htmltext = "30355-02.htm"
			}
		case kristin:
			if cond == 3 {
				if p.HasItem(sentryBlade3) {
					htmltext = "30357-01.htm"
					s.TakeItems(p, sentryBlade3, 1)
					s.GiveItems(p, oldBronzeSword, 1)
					if p.ItemCount(oldBronzeSword) == 2 {
						st.SetCond(4)
						s.PlaySound(p, script.SoundMiddle)
					}
				} else {
					htmltext = "30357-02.htm"
				}
			} else if cond == 4 {
				htmltext = "30357-02.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

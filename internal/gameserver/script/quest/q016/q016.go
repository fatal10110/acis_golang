// Package q016 is the quest The Coming Darkness: the hierarch gives a player
// five crystals of seal, one for each evil altar, visited in order, for
// experience.
package q016

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q016_TheComingDarkness"

// NPCs.
const (
	hierarch   = 31517
	evilAltar1 = 31512
	evilAltar2 = 31513
	evilAltar3 = 31514
	evilAltar4 = 31515
	evilAltar5 = 31516
)

// crystalOfSeal is the quest item.
const crystalOfSeal = 7167

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "The Coming Darkness",
		QuestID: 16,
		Items:   []int32{crystalOfSeal},
		Bind: script.Bindings{
			script.EventQuestStart: {hierarch},
			script.EventTalked:     {hierarch, evilAltar1, evilAltar2, evilAltar3, evilAltar4, evilAltar5},
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
	switch {
	case strings.EqualFold(e.Name, "31517-2.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, crystalOfSeal, 5)
	case strings.EqualFold(e.Name, "31512-1.htm"):
		st.SetCond(2)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.TakeItems(e.Player, crystalOfSeal, 1)
	case strings.EqualFold(e.Name, "31513-1.htm"):
		st.SetCond(3)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.TakeItems(e.Player, crystalOfSeal, 1)
	case strings.EqualFold(e.Name, "31514-1.htm"):
		st.SetCond(4)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.TakeItems(e.Player, crystalOfSeal, 1)
	case strings.EqualFold(e.Name, "31515-1.htm"):
		st.SetCond(5)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.TakeItems(e.Player, crystalOfSeal, 1)
	case strings.EqualFold(e.Name, "31516-1.htm"):
		st.SetCond(6)
		s.PlaySound(e.Player, script.SoundMiddle)
		s.TakeItems(e.Player, crystalOfSeal, 1)
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
		if e.Player.Level() < 62 {
			htmltext = "31517-0a.htm"
		} else {
			htmltext = "31517-0.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		npcID := e.NPC.NpcID()
		switch npcID {
		case hierarch:
			if cond == 6 {
				htmltext = "31517-4.htm"
				s.RewardExpAndSp(e.Player, 221958, 0)
				s.PlaySound(e.Player, script.SoundFinish)
				st.Exit(false)
			} else if e.Player.HasItem(crystalOfSeal) {
				htmltext = "31517-3.htm"
			} else {
				htmltext = "31517-3a.htm"
				st.Exit(true)
			}
		case evilAltar1, evilAltar2, evilAltar3, evilAltar4, evilAltar5:
			condAltar := npcID - 31511
			if cond == condAltar {
				if e.Player.HasItem(crystalOfSeal) {
					htmltext = strconv.Itoa(int(npcID)) + "-0.htm"
				} else {
					htmltext = "altar_nocrystal.htm"
				}
			} else if cond > condAltar {
				htmltext = strconv.Itoa(int(npcID)) + "-2.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

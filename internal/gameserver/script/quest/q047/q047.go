// Package q047 is the quest Into the Dark Forest: for a dark elf holding a mark
// of traveler, Galladucci sends order documents to Gentler, Sandra and
// Dustin, and pays with a special scroll of escape for the necklace.
package q047

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q047_IntoTheDarkForest"

// NPCs.
const (
	galladucci = 30097
	gentler    = 30094
	sandra     = 30090
	dustin     = 30116
)

// Items.
const (
	orderDocument1        = 7563
	orderDocument2        = 7564
	orderDocument3        = 7565
	magicSwordHilt        = 7568
	gemstonePowder        = 7567
	purifiedMagicNecklace = 7566
	markOfTraveler        = 7570
	scrollOfEscapeSpecial = 7556
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Into the Dark Forest",
		QuestID: 47,
		Items:   []int32{orderDocument1, orderDocument2, orderDocument3, magicSwordHilt, gemstonePowder, purifiedMagicNecklace},
		Bind: script.Bindings{
			script.EventQuestStart: {galladucci},
			script.EventTalked:     {galladucci, sandra, dustin, gentler},
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
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30097-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, orderDocument1, 1)
	case strings.EqualFold(e.Name, "30094-02.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, orderDocument1, 1)
		s.GiveItems(p, magicSwordHilt, 1)
	case strings.EqualFold(e.Name, "30097-06.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, magicSwordHilt, 1)
		s.GiveItems(p, orderDocument2, 1)
	case strings.EqualFold(e.Name, "30090-02.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, orderDocument2, 1)
		s.GiveItems(p, gemstonePowder, 1)
	case strings.EqualFold(e.Name, "30097-09.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, gemstonePowder, 1)
		s.GiveItems(p, orderDocument3, 1)
	case strings.EqualFold(e.Name, "30116-02.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, orderDocument3, 1)
		s.GiveItems(p, purifiedMagicNecklace, 1)
	case strings.EqualFold(e.Name, "30097-12.htm"):
		s.TakeItems(p, markOfTraveler, -1)
		s.TakeItems(p, purifiedMagicNecklace, 1)
		s.RewardItems(p, scrollOfEscapeSpecial, 1)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
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
		if p.Race() == player.RaceDarkElf && p.Level() >= 3 {
			if p.HasItem(markOfTraveler) {
				htmltext = "30097-02.htm"
			} else {
				htmltext = "30097-01.htm"
			}
		} else {
			htmltext = "30097-01a.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case galladucci:
			switch cond {
			case 1:
				htmltext = "30097-04.htm"
			case 2:
				htmltext = "30097-05.htm"
			case 3:
				htmltext = "30097-07.htm"
			case 4:
				htmltext = "30097-08.htm"
			case 5:
				htmltext = "30097-10.htm"
			case 6:
				htmltext = "30097-11.htm"
			}
		case gentler:
			if cond == 1 {
				htmltext = "30094-01.htm"
			} else if cond > 1 {
				htmltext = "30094-03.htm"
			}
		case sandra:
			if cond == 3 {
				htmltext = "30090-01.htm"
			} else if cond > 3 {
				htmltext = "30090-03.htm"
			}
		case dustin:
			if cond == 5 {
				htmltext = "30116-01.htm"
			} else if cond == 6 {
				htmltext = "30116-03.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

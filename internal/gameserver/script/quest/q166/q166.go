// Package q166 is the quest Mass of Darkness: a dark elf gathers Iria's
// dagger, Dorankus's wine and Trudy's scripture for Undrias.
package q166

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q166_MassOfDarkness"

// NPCs.
const (
	undrias  = 30130
	iria     = 30135
	dorankus = 30139
	trudy    = 30143
)

// Items.
const (
	undriasLetter    = 1088
	ceremonialDagger = 1089
	dreviantWine     = 1090
	garmielScripture = 1091
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Mass of Darkness",
		QuestID: 166,
		Items:   []int32{undriasLetter, ceremonialDagger, dreviantWine, garmielScripture},
		Bind: script.Bindings{
			script.EventQuestStart: {undrias},
			script.EventTalked:     {undrias, iria, dorankus, trudy},
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
	if strings.EqualFold(e.Name, "30130-04.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, undriasLetter, 1)
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
		case p.Race() != player.RaceDarkElf:
			htmltext = "30130-00.htm"
		case p.Level() < 2:
			htmltext = "30130-02.htm"
		default:
			htmltext = "30130-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case undrias:
			switch cond {
			case 1:
				htmltext = "30130-05.htm"
			case 2:
				htmltext = "30130-06.htm"
				s.TakeItems(p, ceremonialDagger, 1)
				s.TakeItems(p, dreviantWine, 1)
				s.TakeItems(p, garmielScripture, 1)
				s.TakeItems(p, undriasLetter, 1)
				s.RewardItems(p, 57, 500)
				s.RewardExpAndSp(p, 500, 0)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case iria:
			if cond == 1 && !p.HasItem(ceremonialDagger) {
				htmltext = "30135-01.htm"
				s.GiveItems(p, ceremonialDagger, 1)
				if p.HasItems(dreviantWine, garmielScripture) {
					st.SetCond(2)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					s.PlaySound(p, script.SoundItemGet)
				}
			} else if cond == 2 {
				htmltext = "30135-02.htm"
			}
		case dorankus:
			if cond == 1 && !p.HasItem(dreviantWine) {
				htmltext = "30139-01.htm"
				s.GiveItems(p, dreviantWine, 1)
				if p.HasItems(ceremonialDagger, garmielScripture) {
					st.SetCond(2)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					s.PlaySound(p, script.SoundItemGet)
				}
			} else if cond == 2 {
				htmltext = "30139-02.htm"
			}
		case trudy:
			if cond == 1 && !p.HasItem(garmielScripture) {
				htmltext = "30143-01.htm"
				s.GiveItems(p, garmielScripture, 1)
				if p.HasItems(ceremonialDagger, dreviantWine) {
					st.SetCond(2)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					s.PlaySound(p, script.SoundItemGet)
				}
			} else if cond == 2 {
				htmltext = "30143-02.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

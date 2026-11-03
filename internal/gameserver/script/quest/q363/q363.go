// Package q363 is the quest Sorrowful Sound of Flute: Nanarin needs a flute,
// clothes or beer delivered to the performance.
package q363

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q363_SorrowfulSoundOfFlute"

// Npcs.
const (
	nanarin = 30956
	opix    = 30595
	aldo    = 30057
	ranspo  = 30594
	holvas  = 30058
	barbado = 30959
	poitan  = 30458
)

// Quest items.
const (
	nanarinFlute = 4319
	blackBeer    = 4320
	clothes      = 4318
)

// themeOfSolitude is the reward.
const themeOfSolitude = 4420

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Sorrowful Sound of Flute",
		QuestID: 363,
		Items:   []int32{nanarinFlute, blackBeer, clothes},
		Bind: script.Bindings{
			script.EventQuestStart: {nanarin},
			script.EventTalked:     {nanarin, opix, aldo, ranspo, holvas, barbado, poitan},
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
	case strings.EqualFold(e.Name, "30956-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30956-05.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, clothes, 1)
	case strings.EqualFold(e.Name, "30956-06.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, nanarinFlute, 1)
	case strings.EqualFold(e.Name, "30956-07.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, blackBeer, 1)
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
		if p.Level() < 15 {
			htmltext = "30956-03.htm"
		} else {
			htmltext = "30956-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		npcID := e.NPC.NpcID()
		switch npcID {
		case nanarin:
			switch cond {
			case 1:
				htmltext = "30956-02.htm"
			case 2:
				htmltext = "30956-04.htm"
			case 3:
				htmltext = "30956-08.htm"
			case 4:
				if st.GetInt("success") == 1 {
					htmltext = "30956-09.htm"
					s.GiveItems(p, themeOfSolitude, 1)
					s.PlaySound(p, script.SoundFinish)
				} else {
					htmltext = "30956-10.htm"
					s.PlaySound(p, script.SoundGiveUp)
				}
				st.Exit(true)
			}
		case opix, poitan, aldo, ranspo, holvas:
			htmltext = strconv.Itoa(int(npcID)) + "-01.htm"
			if cond == 1 {
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
			}
		case barbado:
			switch cond {
			case 3:
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				if p.HasItem(nanarinFlute) {
					htmltext = "30959-02.htm"
					st.Set("success", strconv.Itoa(1))
				} else {
					htmltext = "30959-01.htm"
				}
				s.TakeItems(p, blackBeer, -1)
				s.TakeItems(p, clothes, -1)
				s.TakeItems(p, nanarinFlute, -1)
			case 4:
				htmltext = "30959-03.htm"
			}
		}
	}
	return htmltext
}

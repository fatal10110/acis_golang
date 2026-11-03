// Package q411 is the quest Path to an Assassin: a dark fighter earns the
// Iron Heart by running Shilen's call through Arkenia and Leikan, bringing
// back a moonstone beast molar and the tears of a monster.
package q411

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q411_PathToAnAssassin"

// Quest items.
const (
	shilenCall            = 1245
	arkeniaLetter         = 1246
	leikanNote            = 1247
	moonstoneBeastMolar   = 1248
	shilenTears           = 1250
	arkeniaRecommendation = 1251
	ironHeart             = 1252
)

// Npcs.
const (
	triskel = 30416
	arkenia = 30419
	leikan  = 30382
)

// Monsters.
const (
	tearsMonster   = 27036
	moonstoneBeast = 20369
)

// The classes the quest checks.
const (
	classDarkFighter = 31
	classAssassin    = 35
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to an Assassin",
		QuestID: 411,
		Items:   []int32{shilenCall, arkeniaLetter, leikanNote, moonstoneBeastMolar, shilenTears, arkeniaRecommendation},
		Bind: script.Bindings{
			script.EventQuestStart: {triskel},
			script.EventTalked:     {triskel, arkenia, leikan},
			script.EventMyDying:    {tearsMonster, moonstoneBeast},
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
	switch {
	case strings.EqualFold(e.Name, "30416-05.htm"):
		switch {
		case p.ClassID() != classDarkFighter:
			if p.ClassID() == classAssassin {
				htmltext = "30416-02a.htm"
			} else {
				htmltext = "30416-02.htm"
			}
		case p.Level() < 19:
			htmltext = "30416-03.htm"
		case p.HasItem(ironHeart):
			htmltext = "30416-04.htm"
		default:
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
			s.GiveItems(p, shilenCall, 1)
		}
	case strings.EqualFold(e.Name, "30419-05.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, shilenCall, 1)
		s.GiveItems(p, arkeniaLetter, 1)
	case strings.EqualFold(e.Name, "30382-03.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, arkeniaLetter, 1)
		s.GiveItems(p, leikanNote, 1)
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
		htmltext = "30416-01.htm"
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case triskel:
			switch {
			case cond == 1:
				htmltext = "30416-11.htm"
			case cond == 2:
				htmltext = "30416-07.htm"
			case cond == 3 || cond == 4:
				htmltext = "30416-08.htm"
			case cond == 5:
				htmltext = "30416-09.htm"
			case cond == 6:
				htmltext = "30416-10.htm"
			case cond == 7:
				htmltext = "30416-06.htm"
				s.TakeItems(p, arkeniaRecommendation, 1)
				s.GiveItems(p, ironHeart, 1)
				s.RewardExpAndSp(p, 3200, 3930)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case arkenia:
			switch {
			case cond == 1:
				htmltext = "30419-01.htm"
			case cond == 2:
				htmltext = "30419-07.htm"
			case cond == 3 || cond == 4:
				htmltext = "30419-10.htm"
			case cond == 5:
				htmltext = "30419-11.htm"
			case cond == 6:
				htmltext = "30419-08.htm"
				st.SetCond(7)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, shilenTears, -1)
				s.GiveItems(p, arkeniaRecommendation, 1)
			case cond == 7:
				htmltext = "30419-09.htm"
			}
		case leikan:
			switch {
			case cond == 2:
				htmltext = "30382-01.htm"
			case cond == 3:
				if !p.HasItem(moonstoneBeastMolar) {
					htmltext = "30382-05.htm"
				} else {
					htmltext = "30382-06.htm"
				}
			case cond == 4:
				htmltext = "30382-07.htm"
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, moonstoneBeastMolar, -1)
				s.TakeItems(p, leikanNote, -1)
			case cond == 5:
				htmltext = "30382-09.htm"
			case cond > 5:
				htmltext = "30382-08.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	if e.NPC.NpcID() == moonstoneBeast {
		if st.Cond() == 3 && s.DropItemsAlways(p, moonstoneBeastMolar, 1, 10) {
			st.SetCond(4)
		}
	} else if st.Cond() == 5 {
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, shilenTears, 1)
	}
}

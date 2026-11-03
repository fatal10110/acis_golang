// Package q037 is the quest Make Formal Wear: Alexis sends the player
// between Leikar, Jeremy and Mist with a signet ring, ice wine and a box of
// cookies, then Leikar takes the cloth, jewel box and sewing kit of the
// helper quests and trades the dress shoes box for formal wear.
package q037

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q037_MakeFormalWear"

// NPCs.
const (
	alexis = 30842
	leikar = 31520
	jeremy = 31521
	mist   = 31627
)

// Items.
const (
	mysteriousCloth = 7076
	jewelBox        = 7077
	sewingKit       = 7078
	dressShoesBox   = 7113
	signetRing      = 7164
	iceWine         = 7160
	boxOfCookies    = 7159
)

// Reward.
const formalWear = 6408

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Make Formal Wear",
		QuestID: 37,
		Items:   []int32{signetRing, iceWine, boxOfCookies},
		Bind: script.Bindings{
			script.EventQuestStart: {alexis},
			script.EventTalked:     {alexis, leikar, jeremy, mist},
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
	case strings.EqualFold(e.Name, "30842-1.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "31520-1.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, signetRing, 1)
	case strings.EqualFold(e.Name, "31521-1.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, signetRing, 1)
		s.GiveItems(p, iceWine, 1)
	case strings.EqualFold(e.Name, "31627-1.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, iceWine, 1)
	case strings.EqualFold(e.Name, "31521-3.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, boxOfCookies, 1)
	case strings.EqualFold(e.Name, "31520-3.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, boxOfCookies, 1)
	case strings.EqualFold(e.Name, "31520-5.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, jewelBox, 1)
		s.TakeItems(p, mysteriousCloth, 1)
		s.TakeItems(p, sewingKit, 1)
	case strings.EqualFold(e.Name, "31520-7.htm"):
		s.TakeItems(p, dressShoesBox, 1)
		s.GiveItems(p, formalWear, 1)
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
		if p.Level() < 60 {
			htmltext = "30842-0a.htm"
		} else {
			htmltext = "30842-0.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case alexis:
			if cond == 1 {
				htmltext = "30842-2.htm"
			}
		case leikar:
			switch {
			case cond == 1:
				htmltext = "31520-0.htm"
			case cond == 2:
				htmltext = "31520-1a.htm"
			case cond == 5 || cond == 6:
				switch {
				case p.HasItems(mysteriousCloth, jewelBox, sewingKit):
					htmltext = "31520-4.htm"
				case p.HasItem(boxOfCookies):
					htmltext = "31520-2.htm"
				default:
					htmltext = "31520-3a.htm"
				}
			case cond == 7:
				if p.HasItem(dressShoesBox) {
					htmltext = "31520-6.htm"
				} else {
					htmltext = "31520-5a.htm"
				}
			}
		case jeremy:
			switch {
			case p.HasItem(signetRing):
				htmltext = "31521-0.htm"
			case cond == 3:
				htmltext = "31521-1a.htm"
			case cond == 4:
				htmltext = "31521-2.htm"
			case cond > 4:
				htmltext = "31521-3a.htm"
			}
		case mist:
			if cond == 3 {
				htmltext = "31627-0.htm"
			} else if cond > 3 {
				htmltext = "31627-2.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

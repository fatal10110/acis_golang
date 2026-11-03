// Package q431 is the quest Wedding March: Kantabilon, for a character of
// level 38 or more, asks for fifty silver crystals from the Lienrik, and
// pays twenty-five wedding echo crystals for them.
package q431

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q431_WeddingMarch"

// kantabilon is the quest giver.
const kantabilon = 31042

// Items.
const (
	silverCrystal      = 7540
	weddingEchoCrystal = 7062
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Wedding March",
		QuestID: 431,
		Items:   []int32{silverCrystal},
		Bind: script.Bindings{
			script.EventQuestStart: {kantabilon},
			script.EventTalked:     {kantabilon},
			script.EventMyDying:    {20786, 20787},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or trades the fifty crystals for the echo
// crystals; with fewer the page says so.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31042-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31042-05.htm"):
		if e.Player.ItemCount(silverCrystal) < 50 {
			htmltext = "31042-03.htm"
		} else {
			s.TakeItems(e.Player, silverCrystal, -1)
			s.GiveItems(e.Player, weddingEchoCrystal, 25)
			s.PlaySound(e.Player, script.SoundFinish)
			st.Exit(true)
		}
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
		if e.Player.Level() < 38 {
			htmltext = "31042-00.htm"
		} else {
			htmltext = "31042-01.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "31042-02.htm"
		case 2:
			if e.Player.ItemCount(silverCrystal) < 50 {
				htmltext = "31042-03.htm"
			} else {
				htmltext = "31042-04.htm"
			}
		}
	}
	return htmltext
}

// onMyDying drops a silver crystal to a party member at cond 1, and moves
// the one who holds fifty to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), silverCrystal, 1, 50, 500000) {
		st.SetCond(2)
	}
}

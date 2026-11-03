// Package q019 is the quest Go to the Pastureland!: Vladimir hands over
// young wild beast meat for Tunatun, who pays adena for it.
package q019

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q019_GoToThePastureland"

// youngWildBeastMeat is the quest item.
const youngWildBeastMeat = 7547

// NPCs.
const (
	vladimir = 31302
	tunatun  = 31537
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Go to the Pastureland!",
		QuestID: 19,
		Items:   []int32{youngWildBeastMeat},
		Bind: script.Bindings{
			script.EventQuestStart: {vladimir},
			script.EventTalked:     {vladimir, tunatun},
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
	case strings.EqualFold(e.Name, "31302-01.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, youngWildBeastMeat, 1)
	case strings.EqualFold(e.Name, "019_finish"):
		if p.HasItem(youngWildBeastMeat) {
			htmltext = "31537-01.htm"
			s.TakeItems(p, youngWildBeastMeat, 1)
			s.RewardItems(p, 57, 30000)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		} else {
			htmltext = "31537-02.htm"
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
		if e.Player.Level() < 63 {
			htmltext = "31302-03.htm"
		} else {
			htmltext = "31302-00.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case vladimir:
			htmltext = "31302-02.htm"
		case tunatun:
			htmltext = "31537-00.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

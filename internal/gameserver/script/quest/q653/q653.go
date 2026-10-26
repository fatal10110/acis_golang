// Package q653 is the quest Wild Maiden: Suki, for a character of level 36 or
// more who gives her a scroll of escape, moves to another place of the land,
// where the character finds Galibredo, who pays for the news.
package q653

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q653_WildMaiden"

// NPCs.
const (
	suki      = 32013
	galibredo = 30181
)

// Items.
const (
	scrollOfEscape = 736
	adena          = 57
)

// spawns are the places Suki moves to.
func spawns() []script.Loc {
	return []script.Loc{
		{X: 66578, Y: 72351, Z: -3731, Heading: 0},
		{X: 77189, Y: 73610, Z: -3708, Heading: 2555},
		{X: 71809, Y: 67377, Z: -3675, Heading: 29130},
		{X: 69166, Y: 88825, Z: -3447, Heading: 43886},
	}
}

// quest holds the place Suki stands at, shared by every player.
type quest struct {
	position *script.IntCell
}

// New returns the quest.
func New() script.Script {
	q := quest{position: new(script.IntCell)}
	return script.Script{
		Title:   "Wild Maiden",
		QuestID: 653,
		Bind: script.Bindings{
			script.EventQuestStart: {suki},
			script.EventTalked:     {galibredo, suki},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnTimer: q.onTimer},
	}
}

// onAdvEvent starts the quest for the scroll, and has Suki move on.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	if strings.EqualFold(e.Name, "32013-03.htm") {
		if p.HasItem(scrollOfEscape) {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
			s.TakeItems(p, scrollOfEscape, 1)

			s.StartTimer("65301", e.NPC, nil, 3000*time.Millisecond)
		} else {
			htmltext = "32013-03a.htm"
			st.Exit(true)
		}
	}
	return htmltext
}

// onTimer moves Suki to a place other than the one she stands at.
func (q quest) onTimer(s *script.Script, e script.Timer) string {
	if strings.EqualFold(e.Name, "65301") {
		places := spawns()
		chance := s.Rnd(4)

		// Never the same place twice.
		for chance == q.position.Get() {
			chance = s.Rnd(4)
		}

		q.position.Set(chance)

		e.NPC.DeleteMe()
		s.Spawner().AddSpawn(suki, places[chance], false, 0)
	}
	return ""
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
		if p.Level() < 36 {
			htmltext = "32013-01.htm"
		} else {
			htmltext = "32013-02.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case galibredo:
			htmltext = "30181-01.htm"
			s.RewardItems(p, adena, 2883)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		case suki:
			htmltext = "32013-04a.htm"
		}
	}
	return htmltext
}

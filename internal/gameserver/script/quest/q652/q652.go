// Package q652 is the quest An Aged Ex-Adventurer: Tantan, for a character of
// level 46 or more who gives him a hundred soulshots of grade C, moves to
// another place of the land, where the character finds his friend Sara for
// adena and perhaps a scroll.
package q652

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q652_AnAgedExAdventurer"

// NPCs.
const (
	tantan = 32012
	sara   = 30180
)

// Items.
const (
	soulshotC     = 1464
	enchantArmorD = 956
	adena         = 57
)

// spawns are the places Tantan moves to.
func spawns() []script.Loc {
	return []script.Loc{
		{X: 78355, Y: -1325, Z: -3659, Heading: 0},
		{X: 79890, Y: -6132, Z: -2922, Heading: 0},
		{X: 90012, Y: -7217, Z: -3085, Heading: 0},
		{X: 94500, Y: -10129, Z: -3290, Heading: 0},
		{X: 96534, Y: -1237, Z: -3677, Heading: 0},
	}
}

// quest holds the place Tantan stands at, shared by every player.
type quest struct {
	position *script.IntCell
}

// New returns the quest.
func New() script.Script {
	q := quest{position: new(script.IntCell)}
	return script.Script{
		Title:   "An Aged Ex-Adventurer",
		QuestID: 652,
		Bind: script.Bindings{
			script.EventQuestStart: {tantan},
			script.EventTalked:     {tantan, sara},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnTimer: q.onTimer},
	}
}

// onAdvEvent starts the quest for the soulshots, and has Tantan move on.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	if strings.EqualFold(e.Name, "32012-02.htm") {
		if p.ItemCount(soulshotC) >= 100 {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
			s.TakeItems(p, soulshotC, 100)

			s.StartTimer("65201", e.NPC, nil, 3000*time.Millisecond)
		} else {
			htmltext = "32012-02a.htm"
			st.Exit(true)
		}
	}
	return htmltext
}

// onTimer moves Tantan to a place other than the one he stands at.
func (q quest) onTimer(s *script.Script, e script.Timer) string {
	if strings.EqualFold(e.Name, "65201") {
		places := spawns()
		chance := s.Rnd(5)

		// Never the same place twice.
		for chance == q.position.Get() {
			chance = s.Rnd(5)
		}

		q.position.Set(chance)

		e.NPC.DeleteMe()
		s.Spawner().AddSpawn(tantan, places[chance], false, 0)
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
		if p.Level() < 46 {
			htmltext = "32012-00.htm"
		} else {
			htmltext = "32012-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case sara:
			if s.Rnd(100) < 50 {
				htmltext = "30180-01.htm"
				s.RewardItems(p, adena, 5026)
				s.GiveItems(p, enchantArmorD, 1)
			} else {
				htmltext = "30180-02.htm"
				s.RewardItems(p, adena, 10000)
			}
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		case tantan:
			htmltext = "32012-04a.htm"
		}
	}
	return htmltext
}

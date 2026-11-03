// Package q651 is the quest Runaway Youth: Ivan asks for a Scroll of
// Escape to go home, leaves for another spot three seconds after he gets
// one, and his father Batidae rewards the player.
package q651

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q651_RunawayYouth"

// NPCs.
const (
	ivan    = 32014
	batidae = 31989
)

// scrollOfEscape is the item Ivan asks for.
const scrollOfEscape = 736

// spawns returns the spots Ivan can stand at, in the order the position
// numbers them.
func spawns() []script.Loc {
	return []script.Loc{
		{X: 118600, Y: -161235, Z: -1119, Heading: 0},
		{X: 108380, Y: -150268, Z: -2376, Heading: 0},
		{X: 123254, Y: -148126, Z: -3425, Heading: 0},
	}
}

// quest holds the spot Ivan last moved to, shared by every player.
type quest struct{ currentPosition *script.IntCell }

// New returns the quest.
func New() script.Script {
	q := quest{currentPosition: new(script.IntCell)}
	return script.Script{
		Title:   "Runaway Youth",
		QuestID: 651,
		Bind: script.Bindings{
			script.EventQuestStart: {ivan},
			script.EventTalked:     {ivan, batidae},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTimer: q.onTimer, OnTalk: onTalk},
	}
}

// onAdvEvent takes the scroll and starts Ivan's three seconds to leave;
// without a scroll the quest ends.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	if strings.EqualFold(e.Name, "32014-04.htm") {
		if e.Player.HasItem(scrollOfEscape) {
			htmltext = "32014-03.htm"
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(e.Player, script.SoundAccept)
			s.TakeItems(e.Player, scrollOfEscape, 1)
			s.StartTimer("65101", e.NPC, nil, 3000*time.Millisecond)
		} else {
			st.Exit(true)
		}
	}
	return htmltext
}

// onTimer moves Ivan: he leaves the world and stands up again at another
// of his spots, drawn until it differs from the last one.
func (q quest) onTimer(s *script.Script, e script.Timer) string {
	if strings.EqualFold(e.Name, "65101") {
		chance := s.Rnd(3)
		for chance == q.currentPosition.Get() {
			chance = s.Rnd(3)
		}
		q.currentPosition.Set(chance)
		e.NPC.DeleteMe()
		s.Spawner().AddSpawn(ivan, spawns()[chance], false, 0)
	}
	return ""
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 26 {
			htmltext = "32014-01.htm"
		} else {
			htmltext = "32014-02.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case batidae:
			htmltext = "31989-01.htm"
			s.RewardItems(e.Player, 57, 2883)
			s.PlaySound(e.Player, script.SoundFinish)
			st.Exit(true)
		case ivan:
			htmltext = "32014-04a.htm"
		}
	}
	return htmltext
}

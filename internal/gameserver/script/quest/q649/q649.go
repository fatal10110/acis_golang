// Package q649 is the quest A Looter and a Railroad Man: Obi, for a character
// of level 30 or more, pays for two hundred marks of the thief guild.
package q649

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q649_ALooterAndARailroadMan"

// Items.
const thiefGuildMark = 8099

// NPC.
const obi = 32052

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "A Looter and a Railroad Man",
		QuestID: 649,
		Items:   []int32{thiefGuildMark},
		Bind: script.Bindings{
			script.EventQuestStart: {obi},
			script.EventTalked:     {obi},
			script.EventMyDying:    {22017, 22018, 22019, 22021, 22022, 22023, 22024, 22026},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or pays adena for the marks once there are
// two hundred of them.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "32052-1.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "32052-3.htm"):
		if e.Player.ItemCount(thiefGuildMark) < 200 {
			htmltext = "32052-3a.htm"
		} else {
			s.TakeItems(e.Player, thiefGuildMark, -1)
			s.RewardItems(e.Player, 57, 21698)
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
		if e.Player.Level() < 30 {
			htmltext = "32052-0a.htm"
		} else {
			htmltext = "32052-0.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "32052-2a.htm"
		case 2:
			htmltext = "32052-2.htm"
		}
	}
	return htmltext
}

// onMyDying drops a mark to the killer at cond 1, and moves the killer to
// cond 2 on the two hundredth.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	if s.DropItems(p, thiefGuildMark, 1, 200, 800000) {
		st.SetCond(2)
	}
}

// Package q432 is the quest Birthday Party Song: Octavia, for a character
// of level 31 or more, asks for fifty red crystals from the Roughly Hewn
// Rock Golems, and pays twenty-five of her items for exactly fifty.
package q432

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q432_BirthdayPartySong"

// octavia is the quest giver.
const octavia = 31043

// Items.
const (
	redCrystal = 7541
	reward     = 7061
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Birthday Party Song",
		QuestID: 432,
		Items:   []int32{redCrystal},
		Bind: script.Bindings{
			script.EventQuestStart: {octavia},
			script.EventTalked:     {octavia},
			script.EventMyDying:    {21103},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or pays the reward to a player who holds
// exactly fifty crystals; with any other number the page stays the one
// asked for.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31043-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31043-06.htm"):
		if e.Player.ItemCount(redCrystal) == 50 {
			htmltext = "31043-05.htm"
			s.TakeItems(e.Player, redCrystal, -1)
			s.RewardItems(e.Player, reward, 25)
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
		if e.Player.Level() < 31 {
			htmltext = "31043-00.htm"
		} else {
			htmltext = "31043-01.htm"
		}
	case questlog.StatusStarted:
		if e.Player.ItemCount(redCrystal) < 50 {
			htmltext = "31043-03.htm"
		} else {
			htmltext = "31043-04.htm"
		}
	}
	return htmltext
}

// onMyDying drops a red crystal to a party member at cond 1, and moves the
// one who holds fifty to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), redCrystal, 1, 50, 500000) {
		st.SetCond(2)
	}
}

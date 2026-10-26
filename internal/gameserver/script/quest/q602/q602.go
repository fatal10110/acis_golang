// Package q602 is the quest Shadow of Light: the Eye of Argos asks a
// character of level 68 or more for the eye of darkness from the Buffalo
// and Grendel Slaves, and pays adena, experience and, by chance, items.
package q602

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q602_ShadowOfLight"

// Quest giver and monsters.
const (
	eyeOfArgos   = 31683
	buffaloSlave = 21299
	grendelSlave = 21304
)

// eyeOfDarkness is the item the slaves drop.
const eyeOfDarkness = 7189

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Shadow of Light",
		QuestID: 602,
		Items:   []int32{eyeOfDarkness},
		Bind: script.Bindings{
			script.EventQuestStart: {eyeOfArgos},
			script.EventTalked:     {eyeOfArgos},
			script.EventMyDying:    {buffaloSlave, grendelSlave},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// reward returns the item, the adena, the experience and the skill points
// of the first row of the reward table that a draw of random under 100
// falls under; item 0 is a row with no item, and ok is false for a draw no
// row takes.
func reward(random int) (item, adena int32, exp int64, sp int32, ok bool) {
	switch {
	case random < 20:
		return 6699, 40000, 120000, 20000, true
	case random < 40:
		return 6698, 60000, 110000, 15000, true
	case random < 50:
		return 6700, 40000, 150000, 10000, true
	case random < 100:
		return 0, 100000, 140000, 11250, true
	}
	return 0, 0, 0, 0, false
}

// onAdvEvent starts the quest for a character of level 68, or pays the
// reward row a draw of 100 picks.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31683-02.htm"):
		if e.Player.Level() < 68 {
			htmltext = "31683-02a.htm"
		} else {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(e.Player, script.SoundAccept)
		}
	case strings.EqualFold(e.Name, "31683-05.htm"):
		s.TakeItems(e.Player, eyeOfDarkness, -1)
		if item, adena, exp, sp, ok := reward(s.Rnd(100)); ok {
			s.RewardItems(e.Player, 57, adena)
			if item != 0 {
				s.GiveItems(e.Player, item, 3)
			}
			s.RewardExpAndSp(e.Player, exp, sp)
		}
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
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
		htmltext = "31683-01.htm"
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "31683-03.htm"
		case 2:
			htmltext = "31683-04.htm"
		}
	}
	return htmltext
}

// onMyDying drops an eye of darkness to a party member at cond 1, and
// moves the one who holds a hundred to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	chance := int32(500000)
	if e.NPC.NpcID() == buffaloSlave {
		chance = 450000
	}
	if s.DropItems(st.Player(), eyeOfDarkness, 1, 100, chance) {
		st.SetCond(2)
	}
}

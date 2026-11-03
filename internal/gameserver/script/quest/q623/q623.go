// Package q623 is the quest The Finest Food: Jeremy, for a character of
// level 71 or more, asks for a hundred each of the leaves of Flava, buffalo
// meat and antelope horns, and pays adena with a recipe or experience.
package q623

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q623_TheFinestFood"

// NPC.
const jeremy = 31521

// Items.
const (
	leafOfFlava  = 7199
	buffaloMeat  = 7200
	antelopeHorn = 7201
)

// Monsters.
const (
	flava    = 21316
	buffalo  = 21315
	antelope = 21318
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "The Finest Food",
		QuestID: 623,
		Items:   []int32{leafOfFlava, buffaloMeat, antelopeHorn},
		Bind: script.Bindings{
			script.EventQuestStart: {jeremy},
			script.EventTalked:     {jeremy},
			script.EventMyDying:    {flava, buffalo, antelope},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest for a character of level 71 or more, or takes
// the food in for one of the four rewards.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31521-02.htm"):
		if e.Player.Level() >= 71 {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(e.Player, script.SoundAccept)
		} else {
			htmltext = "31521-03.htm"
		}
	case strings.EqualFold(e.Name, "31521-05.htm"):
		s.TakeItems(e.Player, leafOfFlava, -1)
		s.TakeItems(e.Player, buffaloMeat, -1)
		s.TakeItems(e.Player, antelopeHorn, -1)

		luck := s.Rnd(100)
		switch {
		case luck < 11:
			s.RewardItems(e.Player, 57, 25000)
			s.GiveItems(e.Player, 6849, 1)
		case luck < 23:
			s.RewardItems(e.Player, 57, 65000)
			s.GiveItems(e.Player, 6847, 1)
		case luck < 33:
			s.RewardItems(e.Player, 57, 25000)
			s.GiveItems(e.Player, 6851, 1)
		default:
			s.RewardItems(e.Player, 57, 73000)
			s.RewardExpAndSp(e.Player, 230000, 18250)
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
		htmltext = "31521-01.htm"
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "31521-06.htm"
		case 2:
			if e.Player.ItemCount(leafOfFlava) >= 100 && e.Player.ItemCount(buffaloMeat) >= 100 && e.Player.ItemCount(antelopeHorn) >= 100 {
				htmltext = "31521-04.htm"
			} else {
				htmltext = "31521-07.htm"
			}
		}
	}
	return htmltext
}

// onMyDying drops one of the foods the monster carries to a party member at
// cond 1, and moves the one who then holds a hundred of each to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	player := st.Player()
	switch e.NPC.NpcID() {
	case flava:
		if s.DropItemsAlways(player, leafOfFlava, 1, 100) && player.ItemCount(buffaloMeat) >= 100 && player.ItemCount(antelopeHorn) >= 100 {
			st.SetCond(2)
		}
	case buffalo:
		if s.DropItemsAlways(player, buffaloMeat, 1, 100) && player.ItemCount(leafOfFlava) >= 100 && player.ItemCount(antelopeHorn) >= 100 {
			st.SetCond(2)
		}
	case antelope:
		if s.DropItemsAlways(player, antelopeHorn, 1, 100) && player.ItemCount(leafOfFlava) >= 100 && player.ItemCount(buffaloMeat) >= 100 {
			st.SetCond(2)
		}
	}
}

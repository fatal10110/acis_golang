// Package q631 is the quest Delicious Top Choice Meat: Tunatun, for a
// character of level 65 or more, asks for a hundred and twenty pieces of top
// quality meat that the monsters of the Beast Farm drop, and pays the
// enchant scroll the page picks.
package q631

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q631_DeliciousTopChoiceMeat"

// NPC.
const tunatun = 31537

// Items.
const topQualityMeat = 7546

// rewards returns the scroll and its count the page picks by index.
func rewards() [][2]int32 {
	return [][2]int32{
		{4039, 15},
		{4043, 15},
		{4044, 15},
		{4040, 10},
		{4042, 10},
		{4041, 5},
	}
}

// chance returns the monster npcID's drop chance of top quality meat.
func chance(npcID int32) int32 {
	switch npcID {
	case 21460:
		return 601000
	case 21461:
		return 480000
	case 21462:
		return 447000
	case 21463:
		return 808000
	case 21464:
		return 447000
	case 21465:
		return 808000
	case 21466:
		return 447000
	case 21467:
		return 808000
	case 21479:
		return 477000
	case 21480:
		return 863000
	case 21481:
		return 477000
	case 21482:
		return 863000
	case 21483:
		return 477000
	case 21484:
		return 863000
	case 21485:
		return 477000
	case 21486:
		return 863000
	case 21498:
		return 509000
	case 21499:
		return 920000
	case 21500:
		return 509000
	case 21501:
		return 920000
	case 21502:
		return 509000
	case 21503:
		return 920000
	case 21504:
		return 509000
	case 21505:
		return 920000
	}
	return 0
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Delicious Top Choice Meat",
		QuestID: 631,
		Items:   []int32{topQualityMeat},
		Bind: script.Bindings{
			script.EventQuestStart: {tunatun},
			script.EventTalked:     {tunatun},
			script.EventMyDying: {
				21460, 21461, 21462, 21463, 21464, 21465, 21466, 21467, 21479, 21480, 21481, 21482,
				21483, 21484, 21485, 21486, 21498, 21499, 21500, 21501, 21502, 21503, 21504, 21505,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest for a character of level 65 or more, or takes
// the meat in for the reward whose index the page sent; the client can only
// send a link of the page it was shown.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31537-03.htm"):
		if e.Player.Level() >= 65 {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(e.Player, script.SoundAccept)
		} else {
			htmltext = "31537-02.htm"
			st.Exit(true)
		}
	case script.IsDigit(e.Name):
		if e.Player.ItemCount(topQualityMeat) >= 120 {
			htmltext = "31537-06.htm"
			s.TakeItems(e.Player, topQualityMeat, -1)

			index, err := strconv.ParseInt(e.Name, 10, 32)
			if err != nil {
				panic(err)
			}
			reward := rewards()[index]
			s.RewardItems(e.Player, reward[0], reward[1])

			s.PlaySound(e.Player, script.SoundFinish)
			st.Exit(true)
		} else {
			st.SetCond(1)
			htmltext = "31537-07.htm"
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
		htmltext = "31537-01.htm"
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "31537-03a.htm"
		case 2:
			if e.Player.ItemCount(topQualityMeat) >= 120 {
				htmltext = "31537-04.htm"
			} else {
				st.SetCond(1)
				htmltext = "31537-03a.htm"
			}
		}
	}
	return htmltext
}

// onMyDying drops top quality meat to a party member at cond 1, and moves
// the one who then holds a hundred and twenty to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), topQualityMeat, 1, 120, chance(e.NPC.NpcID())) {
		st.SetCond(2)
	}
}

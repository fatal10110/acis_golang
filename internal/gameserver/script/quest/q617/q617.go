// Package q617 is the quest Gather the Flames: Vulcan or Hilda, for a
// character of level 74 or more, asks for torches that the monsters of the
// Valley of Saints drop. A thousand torches go to Vulcan for a random
// recipe of an S grade weapon; Rooney takes twelve hundred for the recipe
// the player picks.
package q617

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q617_GatherTheFlames"

// NPCs.
const (
	hilda  = 31271
	vulcan = 31539
	rooney = 32049
)

// Items.
const torch = 7264

// rewards returns the recipes Vulcan hands out.
func rewards() []int32 {
	return []int32{6881, 6883, 6885, 6887, 6891, 6893, 6895, 6897, 6899, 7580}
}

// chance returns the monster npcID's drop chance of a torch.
func chance(npcID int32) int32 {
	switch npcID {
	case 21376:
		return 480000
	case 21377:
		return 480000
	case 21378:
		return 490000
	case 21379:
		return 590000
	case 21380:
		return 490000
	case 21381:
		return 510000
	case 21382:
		return 600000
	case 21383:
		return 510000
	case 21384:
		return 640000
	case 21385:
		return 520000
	case 21386:
		return 520000
	case 21387:
		return 530000
	case 21388:
		return 530000
	case 21389:
		return 550000
	case 21390:
		return 560000
	case 21391:
		return 550000
	case 21392:
		return 560000
	case 21393:
		return 580000
	case 21394:
		return 510000
	case 21395:
		return 560000
	case 21652:
		return 490000
	case 21653:
		return 510000
	case 21654:
		return 520000
	case 21655:
		return 530000
	case 21656:
		return 690000
	case 21657:
		return 570000
	}
	return 0
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Gather the Flames",
		QuestID: 617,
		Items:   []int32{torch},
		Bind: script.Bindings{
			script.EventQuestStart: {vulcan, hilda},
			script.EventTalked:     {vulcan, hilda, rooney},
			script.EventMyDying: {
				21376, 21377, 21378, 21379, 21380, 21381, 21382, 21383, 21384, 21385, 21386, 21387, 21388,
				21389, 21390, 21391, 21392, 21393, 21394, 21395, 21652, 21653, 21654, 21655, 21656, 21657,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, hands a thousand torches in to Vulcan for a
// random recipe, quits the quest, or hands twelve hundred in to Rooney for
// the recipe whose id the page sent; the client can only send a link of the
// page it was shown.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31539-03.htm"), strings.EqualFold(e.Name, "31271-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31539-05.htm"):
		if e.Player.ItemCount(torch) >= 1000 {
			htmltext = "31539-07.htm"
			s.TakeItems(e.Player, torch, 1000)
			rs := rewards()
			s.GiveItems(e.Player, rs[s.Rnd(len(rs))], 1)
		}
	case strings.EqualFold(e.Name, "31539-08.htm"):
		s.TakeItems(e.Player, torch, -1)
		st.Exit(true)
	case script.IsDigit(e.Name):
		if e.Player.ItemCount(torch) >= 1200 {
			id, err := strconv.Atoi(e.Name)
			if err != nil {
				panic(err)
			}
			htmltext = "32049-03.htm"
			s.TakeItems(e.Player, torch, 1200)
			s.GiveItems(e.Player, int32(id), 1)
		} else {
			htmltext = "32049-02.htm"
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
		suffix := "-02.htm"
		if e.Player.Level() >= 74 {
			suffix = "-01.htm"
		}
		htmltext = strconv.Itoa(int(e.NPC.NpcID())) + suffix
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case vulcan:
			if e.Player.ItemCount(torch) >= 1000 {
				htmltext = "31539-04.htm"
			} else {
				htmltext = "31539-05.htm"
			}
		case hilda:
			htmltext = "31271-04.htm"
		case rooney:
			if e.Player.ItemCount(torch) >= 1200 {
				htmltext = "32049-01.htm"
			} else {
				htmltext = "32049-02.htm"
			}
		}
	}
	return htmltext
}

// onMyDying drops a torch to a party member who has the quest.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(st.Player(), torch, 1, 0, chance(e.NPC.NpcID()))
}

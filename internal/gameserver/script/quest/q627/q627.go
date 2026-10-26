// Package q627 is the quest Heart in Search of Power: the Necromancer, for a
// character of level 60 or more, asks for three hundred beads of obedience
// that the monsters of the Valley of Saints drop, trades them for a seal of
// light that Enfeux turns into a gem of Saints, and pays adena with an
// enchant scroll the player picks.
package q627

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q627_HeartInSearchOfPower"

// NPCs.
const (
	necromancer = 31518
	enfeux      = 31519
)

// Items.
const (
	sealOfLight     = 7170
	beadOfObedience = 7171
	gemOfSaints     = 7172
)

// chance returns the monster npcID's drop chance of a bead of obedience.
func chance(npcID int32) int32 {
	switch npcID {
	case 21520:
		return 550000
	case 21523:
		return 584000
	case 21524:
		return 621000
	case 21525:
		return 621000
	case 21526:
		return 606000
	case 21529:
		return 625000
	case 21530:
		return 578000
	case 21531:
		return 690000
	case 21532:
		return 671000
	case 21535:
		return 693000
	case 21536:
		return 615000
	case 21539:
		return 762000
	case 21540:
		return 762000
	case 21658:
		return 690000
	}
	return 0
}

// reward returns the item, its count and the adena of the reward the page
// link event names, and whether it names one.
func reward(event string) (item, count, adena int32, ok bool) {
	switch event {
	case "adena":
		return 0, 0, 100000, true
	case "asofe":
		return 4043, 13, 6400, true
	case "thon":
		return 4044, 13, 6400, true
	case "enria":
		return 4042, 6, 13600, true
	case "mold":
		return 4041, 3, 17200, true
	}
	return 0, 0, 0, false
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Heart in Search of Power",
		QuestID: 627,
		Items:   []int32{beadOfObedience},
		Bind: script.Bindings{
			script.EventQuestStart: {necromancer},
			script.EventTalked:     {necromancer, enfeux},
			script.EventMyDying: {
				21520, 21523, 21524, 21525, 21526, 21529, 21530, 21531, 21532, 21535, 21536, 21539, 21540, 21658,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, trades the beads for the seal, the seal for
// the gem, and the gem for the reward the page link names.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	if item, count, adena, ok := reward(e.Name); ok {
		if e.Player.ItemCount(gemOfSaints) == 1 {
			htmltext = "31518-07.htm"
			s.TakeItems(e.Player, gemOfSaints, 1)
			if item > 0 {
				s.GiveItems(e.Player, item, count)
			}
			s.RewardItems(e.Player, 57, adena)
			s.PlaySound(e.Player, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "31518-7.htm"
		}
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31518-01.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31518-03.htm"):
		if e.Player.ItemCount(beadOfObedience) == 300 {
			st.SetCond(3)
			s.PlaySound(e.Player, script.SoundMiddle)
			s.TakeItems(e.Player, beadOfObedience, -1)
			s.GiveItems(e.Player, sealOfLight, 1)
		} else {
			htmltext = "31518-03a.htm"
			st.SetCond(1)
			s.TakeItems(e.Player, beadOfObedience, -1)
		}
	case strings.EqualFold(e.Name, "31519-01.htm"):
		if e.Player.ItemCount(sealOfLight) == 1 {
			st.SetCond(4)
			s.PlaySound(e.Player, script.SoundMiddle)
			s.TakeItems(e.Player, sealOfLight, 1)
			s.GiveItems(e.Player, gemOfSaints, 1)
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
		if e.Player.Level() < 60 {
			htmltext = "31518-00a.htm"
		} else {
			htmltext = "31518-00.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case necromancer:
			switch cond {
			case 1:
				htmltext = "31518-01a.htm"
			case 2:
				htmltext = "31518-02.htm"
			case 3:
				htmltext = "31518-04.htm"
			case 4:
				htmltext = "31518-05.htm"
			}
		case enfeux:
			switch cond {
			case 3:
				htmltext = "31519-00.htm"
			case 4:
				htmltext = "31519-02.htm"
			}
		}
	}
	return htmltext
}

// onMyDying drops a bead of obedience to a party member at cond 1, and moves
// the one who then holds three hundred to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), beadOfObedience, 1, 300, chance(e.NPC.NpcID())) {
		st.SetCond(2)
	}
}

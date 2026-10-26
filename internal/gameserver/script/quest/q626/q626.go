// Package q626 is the quest A Dark Twilight: the Hierarch, for a character of
// level 60 or more, asks for three hundred bloods of Saint that the monsters
// of the Valley of Saints drop, and pays experience or adena.
package q626

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q626_ADarkTwilight"

// NPC.
const hierarch = 31517

// Items.
const bloodOfSaint = 7169

// chance returns the monster npcID's drop chance of a blood of Saint.
func chance(npcID int32) int32 {
	switch npcID {
	case 21520:
		return 533000
	case 21523:
		return 566000
	case 21524:
		return 603000
	case 21525:
		return 603000
	case 21526:
		return 587000
	case 21529:
		return 606000
	case 21530:
		return 560000
	case 21531:
		return 669000
	case 21532:
		return 651000
	case 21535:
		return 672000
	case 21536:
		return 597000
	case 21539:
		return 739000
	case 21540:
		return 739000
	case 21658:
		return 669000
	}
	return 0
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "A Dark Twilight",
		QuestID: 626,
		Items:   []int32{bloodOfSaint},
		Bind: script.Bindings{
			script.EventQuestStart: {hierarch},
			script.EventTalked:     {hierarch},
			script.EventMyDying: {
				21520, 21523, 21524, 21525, 21526, 21529, 21530, 21531, 21532, 21535, 21536, 21539, 21540, 21658,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or takes exactly three hundred bloods in for
// the reward the page picked.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31517-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "reward1"):
		if e.Player.ItemCount(bloodOfSaint) == 300 {
			htmltext = "31517-07.htm"
			s.TakeItems(e.Player, bloodOfSaint, 300)
			s.RewardExpAndSp(e.Player, 162773, 12500)
			s.PlaySound(e.Player, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "31517-08.htm"
		}
	case strings.EqualFold(e.Name, "reward2"):
		if e.Player.ItemCount(bloodOfSaint) == 300 {
			htmltext = "31517-07.htm"
			s.TakeItems(e.Player, bloodOfSaint, 300)
			s.RewardItems(e.Player, 57, 100000)
			s.PlaySound(e.Player, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "31517-08.htm"
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
			htmltext = "31517-02.htm"
		} else {
			htmltext = "31517-01.htm"
		}
	case questlog.StatusStarted:
		if st.Cond() == 1 {
			htmltext = "31517-05.htm"
		} else {
			htmltext = "31517-04.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// onMyDying drops a blood of Saint to a party member at cond 1, and moves
// the one who then holds three hundred to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	if s.DropItems(st.Player(), bloodOfSaint, 1, 300, chance(e.NPC.NpcID())) {
		st.SetCond(2)
	}
}

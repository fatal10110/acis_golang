// Package q633 is the quest In the Forgotten Village: Mina, for a character of
// level 65 or more, asks for two hundred rib bones that the skeletons of the
// Forgotten Village drop, and pays adena and experience. The zombies drop
// livers too, and no one asks for them.
package q633

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q633_InTheForgottenVillage"

// NPC.
const mina = 31388

// Items.
const (
	ribBone     = 7544
	zombieLiver = 7545
)

// mobChance returns the skeleton npcID's drop chance of a rib bone, and
// whether it is one.
func mobChance(npcID int32) (int32, bool) {
	switch npcID {
	case 21557:
		return 328000, true
	case 21558:
		return 328000, true
	case 21559:
		return 337000, true
	case 21560:
		return 337000, true
	case 21563:
		return 342000, true
	case 21564:
		return 348000, true
	case 21565:
		return 351000, true
	case 21566:
		return 359000, true
	case 21567:
		return 359000, true
	case 21572:
		return 365000, true
	case 21574:
		return 383000, true
	case 21575:
		return 383000, true
	case 21580:
		return 385000, true
	case 21581:
		return 395000, true
	case 21583:
		return 397000, true
	case 21584:
		return 401000, true
	}
	return 0, false
}

// undeadChance returns the zombie npcID's drop chance of a liver, and
// whether it is one.
func undeadChance(npcID int32) (int32, bool) {
	switch npcID {
	case 21553:
		return 347000, true
	case 21554:
		return 347000, true
	case 21561:
		return 450000, true
	case 21578:
		return 501000, true
	case 21596:
		return 359000, true
	case 21597:
		return 370000, true
	case 21598:
		return 441000, true
	case 21599:
		return 395000, true
	case 21600:
		return 408000, true
	case 21601:
		return 411000, true
	}
	return 0, false
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "In the Forgotten Village",
		QuestID: 633,
		Items:   []int32{ribBone, zombieLiver},
		Bind: script.Bindings{
			script.EventQuestStart: {mina},
			script.EventTalked:     {mina},
			script.EventMyDying: {
				21557, 21558, 21559, 21560, 21563, 21564, 21565, 21566, 21567, 21572, 21574, 21575,
				21580, 21581, 21583, 21584,
				21553, 21554, 21561, 21578, 21596, 21597, 21598, 21599, 21600, 21601,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, pays adena and experience for two hundred rib
// bones, or gives the quest up.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31388-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31388-10.htm"):
		s.TakeItems(e.Player, ribBone, -1)
		s.PlaySound(e.Player, script.SoundGiveUp)
		st.Exit(true)
	case strings.EqualFold(e.Name, "31388-09.htm"):
		if e.Player.ItemCount(ribBone) >= 200 {
			htmltext = "31388-08.htm"
			s.TakeItems(e.Player, ribBone, 200)
			s.RewardItems(e.Player, 57, 25000)
			s.RewardExpAndSp(e.Player, 305235, 0)
			s.PlaySound(e.Player, script.SoundFinish)
		}
		st.SetCond(1)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 65 {
			htmltext = "31388-03.htm"
		} else {
			htmltext = "31388-01.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "31388-06.htm"
		case 2:
			htmltext = "31388-05.htm"
		}
	}
	return htmltext
}

// onMyDying drops a liver from a zombie to a party member who has the quest,
// and a rib bone from a skeleton to one at cond 1, who moves to cond 2 on
// the two hundredth.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	npcID := e.NPC.NpcID()
	if chance, ok := undeadChance(npcID); ok {
		st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
		if st == nil {
			return
		}
		s.DropItems(st.Player(), zombieLiver, 1, 0, chance)
	} else if chance, ok := mobChance(npcID); ok {
		st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
		if st == nil {
			return
		}
		if s.DropItems(st.Player(), ribBone, 1, 200, chance) {
			st.SetCond(2)
		}
	}
}

// Package q624 is the quest The Finest Ingredients, part 1: Jeremy, for a
// character of level 73 or more, asks for fifty each of the trunks of
// Nepenthes, the feet of Bandersnatchlings and secret spices, and pays an
// ice crystal and a soy sauce jar.
package q624

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q624_TheFinestIngredients_Part1"

// NPC.
const jeremy = 31521

// Monsters.
const (
	nepenthes    = 21319
	atrox        = 21321
	atroxSpawn   = 21317
	bandersnatch = 21314
)

// Items.
const (
	trunkOfNepenthes       = 7202
	footOfBandersnatchling = 7203
	secretSpice            = 7204
	iceCrystal             = 7080
	soySauceJar            = 7205
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "The Finest Ingredients - Part 1",
		QuestID: 624,
		Items:   []int32{trunkOfNepenthes, footOfBandersnatchling, secretSpice},
		Bind: script.Bindings{
			script.EventQuestStart: {jeremy},
			script.EventTalked:     {jeremy},
			script.EventMyDying:    {nepenthes, atrox, atroxSpawn, bandersnatch},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, or takes the ingredients in for the rewards;
// a character short of them goes back to cond 1.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "31521-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31521-05.htm"):
		if holdsAll(e.Player) {
			s.TakeItems(e.Player, trunkOfNepenthes, -1)
			s.TakeItems(e.Player, footOfBandersnatchling, -1)
			s.TakeItems(e.Player, secretSpice, -1)
			s.GiveItems(e.Player, iceCrystal, 1)
			s.GiveItems(e.Player, soySauceJar, 1)
			s.PlaySound(e.Player, script.SoundFinish)
			st.Exit(true)
		} else {
			st.SetCond(1)
			htmltext = "31521-07.htm"
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
		if e.Player.Level() < 73 {
			htmltext = "31521-03.htm"
		} else {
			htmltext = "31521-01.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "31521-06.htm"
		case 2:
			if holdsAll(e.Player) {
				htmltext = "31521-04.htm"
			} else {
				htmltext = "31521-07.htm"
			}
		}
	}
	return htmltext
}

// onMyDying drops one of the ingredients the monster carries to a party
// member at cond 1, and moves the one who then holds fifty of each to cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMember(p, e.NPC, script.ByCond(1))
	if st == nil {
		return
	}
	player := st.Player()
	switch e.NPC.NpcID() {
	case nepenthes:
		if s.DropItemsAlways(player, trunkOfNepenthes, 1, 50) && player.ItemCount(footOfBandersnatchling) >= 50 && player.ItemCount(secretSpice) >= 50 {
			st.SetCond(2)
		}
	case atrox, atroxSpawn:
		if s.DropItemsAlways(player, secretSpice, 1, 50) && player.ItemCount(trunkOfNepenthes) >= 50 && player.ItemCount(footOfBandersnatchling) >= 50 {
			st.SetCond(2)
		}
	case bandersnatch:
		if s.DropItemsAlways(player, footOfBandersnatchling, 1, 50) && player.ItemCount(trunkOfNepenthes) >= 50 && player.ItemCount(secretSpice) >= 50 {
			st.SetCond(2)
		}
	}
}

// holdsAll reports whether p holds fifty of each ingredient.
func holdsAll(p *script.Player) bool {
	return p.ItemCount(trunkOfNepenthes) >= 50 && p.ItemCount(footOfBandersnatchling) >= 50 && p.ItemCount(secretSpice) >= 50
}

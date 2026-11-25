package bingo

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q386Name = "Q386_StolenDignity"

// Npc.
const romp = 30843

// Quest item.
const stolenInfernium = 6363

// q386Chances returns the ore drop chance of each monster, by NPC id.
func q386Chances() map[int32]int32 {
	return map[int32]int32{
		20970: 208000, // Soldier of Ancient Times
		20971: 299000, // Warrior of Ancient Times
		20958: 170000, // Death Agent
		20960: 149000, // Bloody Ghost
		20963: 199000, // Bloody Lord
		20670: 202000, // Crimson Drake
		21114: 352000, // Cursed Guardian
		20959: 273000, // Dark Guard
		21020: 478000, // Fallen Orc Shaman
		21258: 487000, // Fallen Orc Shaman (trans)
		21003: 173000, // Grave Lich
		20969: 205000, // Giant's Shadow
		21108: 245000, // Glow Wisp
		21005: 211000, // Grave Predator
		21116: 487000, // Hames Orc Overlord
		21113: 370000, // Hames Orc Sniper
		20954: 184000, // Hungered Corpse
		20671: 211000, // Kadios
		21110: 260000, // Marsh Predator
		20967: 257000, // Past Creature
		20956: 216000, // Past Knight
		21021: 234000, // Sharp Talon Tiger
		21259: 487000, // Fallen Orc Shaman
		20974: 440000, // Spiteful Soul Leader
		20975: 390000, // Spiteful Soul Wizard
		21001: 214000, // Wretched Archer
	}
}

// q386Rewards returns the items the board pays.
func q386Rewards() []int32 {
	return []int32{
		5529, 5532, 5533, 5534, 5535, 5536, 5537, 5538, 5539, 5541, 5542, 5543, 5544, 5545, 5546, 5547, 5548,
		8331, 8341, 8342, 8349, 8346,
	}
}

// Q386 is the quest Stolen Dignity: Romp buys the infernium ore of
// the monsters for a game of picking six cells of a scrambled board, and
// pays a random reward by the lines the picks complete.
func Q386() script.Script {
	return script.Script{
		Title:   "Stolen Dignity",
		QuestID: 386,
		Bind: script.Bindings{
			script.EventQuestStart: {romp},
			script.EventTalked:     {romp},
			script.EventMyDying: {
				20970, 20971, 20958, 20960, 20963, 20670, 21114, 20959, 21020, 21258, 21003, 20969, 21108,
				21005, 21116, 21113, 20954, 20671, 21110, 20967, 20956, 21021, 21259, 20974, 20975, 21001,
			},
		},
		Hooks: script.Hooks{OnEvent: q386Event, OnTalk: q386Talk, OnMyDying: q386Dying},
	}
}

func q386Event(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q386Name)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30843-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30843-08.htm"):
		s.PlaySound(p, script.SoundGiveUp)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30843-12.htm"):
		if p.ItemCount(stolenInfernium) < 100 {
			htmltext = "30843-11.htm"
		} else {
			st.Set("board", s.Scramble("123456789"))
			s.TakeItems(p, stolenInfernium, 100)
		}
	case strings.HasPrefix(e.Name, "select_1-"): // first pick
		// Register the first char.
		st.Set("playerArray", e.Name[9:])

		// Send back the finalized HTM with dynamic content.
		htmltext = fillBoard(st, s.HTMLText("30843-13.htm"))
	case strings.HasPrefix(e.Name, "select_2-"): // pick #2-5
		// Stores the current event for future use.
		number := e.Name[9:]

		// Restore the player array.
		playerArray := variable(st, "playerArray")

		// Verify if the given number is already on the player array, if yes, it's invalid, otherwise register it.
		if strings.Contains(playerArray, number) {
			htmltext = fillBoard(st, s.HTMLText("30843-"+strconv.Itoa(13+2*len(playerArray))+".htm"))
		} else {
			// Stores the final String.
			st.Set("playerArray", playerArray+number)

			htmltext = fillBoard(st, s.HTMLText("30843-"+strconv.Itoa(12+2*len(playerArray))+".htm"))
		}
	case strings.HasPrefix(e.Name, "select_3-"): // pick #6
		// Stores the current event for future use.
		number := e.Name[9:]

		// Restore the player array.
		playerArray := variable(st, "playerArray")

		// Verify if the given number is already on the player array, if yes, it's invalid, otherwise calculate reward.
		if strings.Contains(playerArray, number) {
			htmltext = fillBoard(st, s.HTMLText("30843-25.htm"))
		} else {
			// The sixth pick completes the picks and is not stored.
			playerChoice := playerArray + number

			board := strings.Split(variable(st, "board"), "")

			switch winningLines(board, playerChoice) {
			case 3:
				htmltext = s.HTMLText("30843-22.htm")
				table := q386Rewards()
				s.GiveItems(p, table[s.Rnd(len(table))], 4)
			case 0:
				htmltext = s.HTMLText("30843-24.htm")
				table := q386Rewards()
				s.GiveItems(p, table[s.Rnd(len(table))], 10)
			default:
				htmltext = s.HTMLText("30843-23.htm")
			}

			htmltext = reveal(htmltext, board, playerChoice)
		}
	}
	return htmltext
}

func q386Talk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, q386Name)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 58 {
			htmltext = "30843-04.htm"
		} else {
			htmltext = "30843-01.htm"
		}
	case questlog.StatusStarted:
		if p.ItemCount(stolenInfernium) < 100 {
			htmltext = "30843-06.htm"
		} else {
			htmltext = "30843-07.htm"
		}
	}
	return htmltext
}

func q386Dying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMemberState(script.ActingPlayer(e.Killer), e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(st.Player(), stolenInfernium, 1, 0, q386Chances()[e.NPC.NpcID()])
}

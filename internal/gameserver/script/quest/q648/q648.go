// Package q648 is the quest An Ice Merchant's Dream: Rafforty, for a
// character of level 53 or more, buys the ice crystals of the frozen
// monsters and sells goods for them, and the ice shelf turns a silver crystal
// into a black one for whoever guesses its tool.
package q648

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q648_AnIceMerchantsDream"

// completedQuest is the quest that opens Rafforty's full dialog.
const completedQuest = "Q115_TheOtherSideOfTruth"

// Items.
const (
	silverHemocyte   = 8057
	silverIceCrystal = 8077
	blackIceCrystal  = 8078
)

// exCondVariable is the quest variable that keeps the ice shelf's secret tool
// (tens) and the tools tried (units).
const exCondVariable = "exCond"

// NPCs.
const (
	rafforty = 32020
	iceShelf = 32023
)

// rewards returns the crystal, its price and the item the page picks by
// letter.
func rewards() map[string][]int32 {
	return map[string][]int32{
		"a": {silverIceCrystal, 23, 1894},
		"b": {silverIceCrystal, 6, 1881},
		"c": {silverIceCrystal, 8, 1880},
		"d": {blackIceCrystal, 1800, 729},
		"e": {blackIceCrystal, 240, 730},
		"f": {blackIceCrystal, 500, 947},
		"g": {blackIceCrystal, 80, 948},
	}
}

// chances returns the drop chances of a monster, of the silver ice crystal
// and of the silver hemocyte, out of a million.
func chances(npcID int32) (crystal, hemocyte int32) {
	switch npcID {
	case 22080:
		return 285000, 48000
	case 22081:
		return 443000, 0
	case 22082:
		return 510000, 0
	case 22084:
		return 477000, 49000
	case 22085:
		return 420000, 43000
	case 22086:
		return 490000, 50000
	case 22087:
		return 787000, 81000
	case 22088:
		return 480000, 49000
	case 22089:
		return 550000, 56000
	case 22090:
		return 570000, 58000
	case 22092:
		return 623000, 0
	case 22093:
		return 910000, 93000
	case 22094:
		return 553000, 57000
	case 22096:
		return 593000, 61000
	case 22097:
		return 693000, 71000
	case 22098:
		return 717000, 74000
	}
	return 0, 0
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "An Ice Merchant's Dream",
		QuestID: 648,
		Items:   []int32{silverHemocyte, silverIceCrystal, blackIceCrystal},
		Bind: script.Bindings{
			script.EventQuestStart: {rafforty, iceShelf},
			script.EventTalked:     {rafforty, iceShelf},
			script.EventMyDying: {
				22080, 22081, 22082, 22084, 22085, 22086, 22087, 22088, 22089, 22090, 22092, 22093,
				22094, 22096, 22097, 22098,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent runs the pages of Rafforty (start, sell the crystals, buy goods
// for them, end the quest) and of the ice shelf (draw the secret tool, spend
// a silver crystal on a tool, and test the guess).
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Rafforty.
	case strings.EqualFold(e.Name, "32020-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "32020-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(2)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "32020-14.htm"), strings.EqualFold(e.Name, "32020-15.htm"):
		black := p.ItemCount(blackIceCrystal)
		silver := p.ItemCount(silverIceCrystal)
		if silver+black > 0 {
			s.TakeItems(p, blackIceCrystal, -1)
			s.TakeItems(p, silverIceCrystal, -1)
			s.RewardItems(p, 57, silver*300+black*1200)
		} else {
			htmltext = "32020-16a.htm"
		}
	case strings.HasPrefix(e.Name, "32020-17"):
		reward := rewards()[e.Name[8:9]]
		if p.ItemCount(reward[0]) >= reward[1] {
			s.TakeItems(p, reward[0], reward[1])
			s.RewardItems(p, reward[2], 1)
		} else {
			htmltext = "32020-15a.htm"
		}
	case strings.EqualFold(e.Name, "32020-20.htm"), strings.EqualFold(e.Name, "32020-22.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	// Ice shelf.
	case strings.EqualFold(e.Name, "32023-05.htm"):
		if st.GetInt(exCondVariable) == 0 {
			st.Set(exCondVariable, strconv.Itoa((s.Rnd(4)+1)*10))
		}
	case strings.HasPrefix(e.Name, "32023-06-"):
		exCond := st.GetInt(exCondVariable)
		if exCond > 0 {
			htmltext = "32023-06.htm"
			if strings.HasSuffix(e.Name, "chisel") {
				exCond++
			} else {
				exCond += 2
			}
			st.Set(exCondVariable, strconv.Itoa(int(exCond)))
			s.PlaySound(p, "ItemSound2.broken_key")
			s.TakeItems(p, silverIceCrystal, 1)
		}
	case strings.HasPrefix(e.Name, "32023-07-"):
		exCond := st.GetInt(exCondVariable)
		if exCond > 0 {
			val := exCond / 10
			tool := int32(2)
			if strings.HasSuffix(e.Name, "knife") {
				tool = 0
			}
			if val == exCond-val*10+tool {
				htmltext = "32023-07.htm"
				s.PlaySound(p, "ItemSound3.sys_enchant_success")
				s.RewardItems(p, blackIceCrystal, 1)
			} else {
				htmltext = "32023-08.htm"
				s.PlaySound(p, "ItemSound3.sys_enchant_failed")
			}
			st.Set(exCondVariable, strconv.Itoa(0))
		}
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if e.NPC.NpcID() == rafforty {
			if p.Level() < 53 {
				htmltext = "32020-01.htm"
			} else {
				st2 := s.QuestState(p, completedQuest)
				if st2 != nil && st2.IsCompleted() {
					htmltext = "32020-02.htm"
				} else {
					htmltext = "32020-03.htm"
				}
			}
		} else {
			htmltext = "32023-01.htm"
		}
	case questlog.StatusStarted:
		if e.NPC.NpcID() == rafforty {
			hasItem := p.HasAtLeastOneItem(silverIceCrystal, blackIceCrystal)
			st2 := s.QuestState(p, completedQuest)
			if st2 != nil && st2.IsCompleted() {
				if hasItem {
					htmltext = "32020-11.htm"
				} else {
					htmltext = "32020-09.htm"
				}
				if st.Cond() == 1 {
					st.SetCond(2)
					s.PlaySound(p, script.SoundMiddle)
				}
			} else if hasItem {
				htmltext = "32020-10.htm"
			} else {
				htmltext = "32020-08.htm"
			}
		} else if !p.HasItem(silverIceCrystal) {
			htmltext = "32023-02.htm"
		} else if st.GetInt(exCondVariable)%10 == 0 {
			htmltext = "32023-03.htm"
			st.Set(exCondVariable, strconv.Itoa(0))
		} else {
			htmltext = "32023-04.htm"
		}
	}
	return htmltext
}

// onMyDying drops a silver ice crystal to a party member who has the quest,
// and a silver hemocyte too when the member is at cond 2.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	crystal, hemocyte := chances(e.NPC.NpcID())
	s.DropItems(st.Player(), silverIceCrystal, 1, 0, crystal)
	if st.Cond() == 2 && hemocyte > 0 {
		s.DropItems(st.Player(), silverHemocyte, 1, 0, hemocyte)
	}
}

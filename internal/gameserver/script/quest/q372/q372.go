// Package q372 is the quest Legacy of Insolence: Walderal trades blueprints
// for the parts and recipes of the Ancient Empire, and four more collectors
// trade the scrolls the monsters leave.
package q372

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q372_LegacyOfInsolence"

// NPCs.
const (
	walderal = 30844
	patrin   = 30929
	holly    = 30839
	claudia  = 31001
	desmond  = 30855
)

// adena is the money item.
const adena = 57

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Legacy of Insolence",
		QuestID: 372,
		Bind: script.Bindings{
			script.EventQuestStart: {walderal},
			script.EventTalked:     {walderal, patrin, holly, claudia, desmond},
			script.EventMyDying:    {20817, 20821, 20825, 20829, 21069, 21063},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30844-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30844-05b.htm"):
		if st.Cond() == 1 {
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
		}
	case strings.EqualFold(e.Name, "30844-07.htm"):
		for blueprint := int32(5989); blueprint <= 6001; blueprint++ {
			if !p.HasItem(blueprint) {
				htmltext = "30844-06.htm"
				break
			}
		}
	case strings.HasPrefix(e.Name, "30844-07-"):
		if !checkAndTakeItems(s, p, 0) {
			return strconv.Itoa(int(e.NPC.NpcID())) + "-07a.htm"
		}
		chance := s.Rnd(100)
		switch digit(e.Name, 9) {
		case 0:
			blueprints(s, p, chance, [3]int{10, 20, 30}, [3]int{51, 62, 79}, 40, [3]int32{5496, 5508, 5525}, [3]int32{5368, 5392, 5426})
		case 1:
			blueprints(s, p, chance, [3]int{10, 20, 30}, [3]int{51, 62, 79}, 40, [3]int32{5497, 5509, 5526}, [3]int32{5370, 5394, 5428})
		case 2:
			blueprints(s, p, chance, [3]int{17, 34, 49}, [3]int{70, 82, 92}, 58, [3]int32{5502, 5514, 5527}, [3]int32{5380, 5404, 5430})
		case 3:
			blueprints(s, p, chance, [3]int{17, 34, 49}, [3]int{70, 82, 92}, 58, [3]int32{5503, 5515, 5528}, [3]int32{5382, 5406, 5432})
		}
	case strings.EqualFold(e.Name, "30844-09.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	}
	return htmltext
}

// digit reads the character of event at index as a decimal digit; a missing or
// other character panics, so the invocation aborts.
func digit(event string, index int) int {
	n, err := strconv.Atoi(event[index : index+1])
	if err != nil {
		panic(err)
	}
	return n
}

// pay gives one roll of an exchange: each of the three items alone below its
// own bound, all three below allBelow. It reports whether the roll paid.
func pay(s *script.Script, p *script.Player, chance int, bounds [3]int, allBelow int, items [3]int32) bool {
	switch {
	case chance < bounds[0]:
		s.RewardItems(p, items[0], 1)
	case chance < bounds[1]:
		s.RewardItems(p, items[1], 1)
	case chance < bounds[2]:
		s.RewardItems(p, items[2], 1)
	case chance < allBelow:
		rewardAll(s, p, items)
	default:
		return false
	}
	return true
}

// blueprints pays one roll of a blueprint exchange: the parts, then the recipes.
func blueprints(s *script.Script, p *script.Player, chance int, parts, recipes [3]int, partsAllBelow int, partItems, recipeItems [3]int32) {
	if !pay(s, p, chance, parts, partsAllBelow, partItems) {
		pay(s, p, chance, recipes, 100, recipeItems)
	}
}

// scrolls pays one roll of a scroll exchange: the items, then adena.
func scrolls(s *script.Script, p *script.Player, chance int, bounds [3]int, allBelow int, items [3]int32) {
	if !pay(s, p, chance, bounds, allBelow, items) {
		s.RewardItems(p, adena, 4000)
	}
}

func rewardAll(s *script.Script, p *script.Player, items [3]int32) {
	for _, item := range items {
		s.RewardItems(p, item, 1)
	}
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 59 {
			htmltext = "30844-01.htm"
		} else {
			htmltext = "30844-02.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case walderal:
			htmltext = "30844-05.htm"
		case holly:
			if !checkAndTakeItems(s, p, 1) {
				return "30839-01.htm"
			}
			scrolls(s, p, s.Rnd(100), [3]int{30, 60, 80}, 90, [3]int32{5496, 5508, 5525})
			htmltext = "30839-02.htm"
		case patrin:
			if !checkAndTakeItems(s, p, 2) {
				return "30929-01.htm"
			}
			scrolls(s, p, s.Rnd(100), [3]int{30, 60, 80}, 90, [3]int32{5497, 5509, 5526})
			htmltext = "30929-02.htm"
		case claudia:
			if !checkAndTakeItems(s, p, 3) {
				return "31001-01.htm"
			}
			scrolls(s, p, s.Rnd(100), [3]int{31, 62, 75}, 83, [3]int32{5502, 5514, 5527})
			htmltext = "31001-02.htm"
		case desmond:
			if !checkAndTakeItems(s, p, 4) {
				return "30855-01.htm"
			}
			scrolls(s, p, s.Rnd(100), [3]int{31, 62, 75}, 83, [3]int32{5503, 5515, 5528})
			htmltext = "30855-02.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMemberState(script.ActingPlayer(e.Killer), e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	var parchment, rate int32
	switch e.NPC.NpcID() {
	case 20817:
		parchment, rate = 5966, 302000
	case 20821:
		parchment, rate = 5966, 410000
	case 20825:
		parchment, rate = 5966, 447000
	case 20829:
		parchment, rate = 5967, 451000
	case 21069:
		parchment, rate = 5968, 280000
	case 21063:
		parchment, rate = 5969, 290000
	default:
		return
	}
	s.DropItems(st.Player(), parchment, 1, 0, rate)
}

// checkAndTakeItems takes one of every item of a collector's set when the
// player holds the whole set. The sets are, by collector: Walderal's blueprints,
// Holly's genealogies, Patrin's epics, and the revelations of Claudia and
// Desmond.
func checkAndTakeItems(s *script.Script, p *script.Player, itemType int) bool {
	var first, last int32
	switch itemType {
	case 0:
		first, last = 5989, 6001
	case 1:
		first, last = 5984, 5988
	case 2:
		first, last = 5979, 5983
	default:
		first, last = 5972, 5978
	}
	for item := first; item <= last; item++ {
		if !p.HasItem(item) {
			return false
		}
	}
	for item := first; item <= last; item++ {
		s.TakeItems(p, item, 1)
	}
	return true
}

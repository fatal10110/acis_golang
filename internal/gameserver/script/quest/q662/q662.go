// Package q662 is the quest A Game Of Cards: Klump, for a character of level
// 61 or more, trades fifty red gems for a hand of five face-down cards. The
// player turns them over one by one, and the pairs among them decide the
// prize.
package q662

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q662_AGameOfCards"

// NPC.
const klump = 30845

// Quest item.
const redGem = 8765

// Prizes.
const (
	scrollS      = 959  // enchant weapon, S grade
	scrollA      = 729  // enchant weapon, A grade
	scrollB      = 947  // enchant weapon, B grade
	scrollC      = 951  // enchant weapon, C grade
	scrollD      = 955  // enchant weapon, D grade
	armorScrollD = 956  // enchant armor, D grade
	ziggoGem     = 8868 // Ziggo gemstone
)

// drop is a monster that drops a red gem, with its chance out of a million.
type drop struct{ npc, chance int32 }

// drops lists the monsters that drop red gems.
func drops() []drop {
	return []drop{
		{18001, 232000},
		{20672, 357000},
		{20673, 373000},
		{20674, 583000},
		{20677, 435000},
		{20955, 358000},
		{20958, 283000},
		{20959, 455000},
		{20961, 365000},
		{20962, 348000},
		{20965, 457000},
		{20966, 493000},
		{20968, 418000},
		{20972, 350000},
		{20973, 453000},
		{21002, 315000},
		{21004, 320000},
		{21006, 335000},
		{21008, 462000},
		{21010, 397000},
		{21109, 507000},
		{21112, 552000},
		{21114, 587000},
		{21116, 812000},
		{21278, 483000},
		{21279, 483000},
		{21280, 483000},
		{21281, 483000},
		{21286, 515000},
		{21287, 515000},
		{21288, 515000},
		{21289, 515000},
		{21508, 493000},
		{21510, 527000},
		{21513, 562000},
		{21515, 598000},
		{21520, 458000},
		{21526, 552000},
		{21530, 488000},
		{21535, 573000},
	}
}

// New returns the quest.
func New() script.Script {
	monsters := make([]int32, 0, len(drops()))
	for _, d := range drops() {
		monsters = append(monsters, d.npc)
	}
	return script.Script{
		Title:   "A Game Of Cards",
		QuestID: 662,
		Items:   []int32{redGem},
		Bind: script.Bindings{
			script.EventQuestStart: {klump},
			script.EventTalked:     {klump},
			script.EventMyDying:    monsters,
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// card is the face of card number n; 0 is the back.
func card(n int32) string {
	switch n {
	case 0:
		return "?"
	case 1:
		return "!"
	case 2:
		return "="
	case 3:
		return "T"
	case 4:
		return "V"
	case 5:
		return "O"
	case 6:
		return "P"
	case 7:
		return "S"
	case 8:
		return "E"
	case 9:
		return "H"
	case 10:
		return "A"
	case 11:
		return "R"
	case 12:
		return "D"
	case 13:
		return "I"
	case 14:
		return "N"
	}
	return ""
}

// hand is the five cards of a game and the bit set of those turned over.
type hand struct {
	cards  [5]int32
	turned int32
}

// readHand decodes the quest's "state" (the first four cards, two decimal
// digits each) and "stateEx" (the fifth card, and the turned bits above it).
func readHand(state, stateEx int32) hand {
	return hand{
		cards: [5]int32{
			state % 100,
			state % 10000 / 100,
			state % 1000000 / 10000,
			state % 100000000 / 1000000,
			stateEx % 100,
		},
		turned: stateEx / 100,
	}
}

// fill writes the hand into the page's cells and their font colors: a card
// not yet turned shows its back.
func (h hand) fill(page string) string {
	page = h.cell(page, 0, 2, "%FontColor1%", "%Cell1%")
	page = h.cell(page, 1, 4, "%FontColor2%", "%Cell2%")
	page = h.cell(page, 2, 8, "%FontColor3%", "%Cell3%")
	page = h.cell(page, 3, 16, "%FontColor4%", "%Cell4%")
	return h.cell(page, 4, 32, "%FontColor5%", "%Cell5%")
}

// cell fills the font color and the face of card n, whose bit of the turned
// cards is below mod/2 when it is face down.
func (h hand) cell(page string, n int, mod int32, fontColor, cell string) string {
	if h.turned%mod < mod/2 {
		page = strings.ReplaceAll(page, fontColor, "ffff00")
		return strings.ReplaceAll(page, cell, card(0))
	}
	page = strings.ReplaceAll(page, fontColor, "ff6f6f")
	return strings.ReplaceAll(page, cell, card(h.cards[n]))
}

// score is the prize class of a hand: tens for each pair among the first
// card and the rest, then among the others, with a single unit for a pair
// found after a first pair stopped scoring tens.
func (h hand) score() int32 {
	c := h.cards
	for _, v := range c {
		if v < 1 || v > 14 {
			return 0
		}
	}
	var i6, i8 int32
	if c[0] == c[1] {
		i6 += 10
		i8 += 8
	}
	if c[0] == c[2] {
		i6 += 10
		i8 += 4
	}
	if c[0] == c[3] {
		i6 += 10
		i8 += 2
	}
	if c[0] == c[4] {
		i6 += 10
		i8++
	}
	// pairAdd is what a pair of a block adds, decided when the block starts:
	// ten when the score is below ten tens, one when it is a multiple of ten,
	// nothing otherwise.
	pairAdd := func() int32 {
		if i6%100 < 10 {
			return 10
		}
		if i6%10 == 0 {
			return 1
		}
		return 0
	}
	if add := pairAdd(); add != 0 && i8%16 < 8 {
		if i8%8 < 4 && c[1] == c[2] {
			i6 += add
			i8 += 4
		}
		if i8%4 < 2 && c[1] == c[3] {
			i6 += add
			i8 += 2
		}
		if i8%2 < 1 && c[1] == c[4] {
			i6 += add
			i8++
		}
	}
	if add := pairAdd(); add != 0 && i8%8 < 4 {
		if i8%4 < 2 && c[2] == c[3] {
			i6 += add
			i8 += 2
		}
		if i8%2 < 1 && c[2] == c[4] {
			i6 += add
			i8++
		}
	}
	if add := pairAdd(); add != 0 && i8%4 < 2 && i8%2 < 1 && c[3] == c[4] {
		i6 += add
	}
	return i6
}

// onAdvEvent runs the dialog: it starts and ends the quest, deals a hand for
// fifty red gems, turns the cards over and pays the prize of a full hand.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30845-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		setInt(st, "state", 0)
		setInt(st, "stateEx", 0)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30845-04.htm"):
		state := st.GetInt("state")
		stateEx := st.GetInt("stateEx")
		if state == 0 && stateEx == 0 && p.ItemCount(redGem) >= 50 {
			htmltext = "30845-05.htm"
		}
	case strings.EqualFold(e.Name, "30845-07.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30845-11.htm"):
		state := st.GetInt("state")
		stateEx := st.GetInt("stateEx")
		if state == 0 && stateEx == 0 && p.ItemCount(redGem) >= 50 {
			i1 := deal(s)
			i2 := deal(s)
			i3 := deal(s)
			i4 := deal(s)
			i5 := deal(s)
			setInt(st, "state", i4*1000000+i3*10000+i2*100+i1)
			setInt(st, "stateEx", i5)
			s.TakeItems(p, redGem, 50)
		}
	case e.Name == "First", e.Name == "Second", e.Name == "Third", e.Name == "Fourth", e.Name == "Fifth":
		h := readHand(st.GetInt("state"), st.GetInt("stateEx"))
		switch e.Name {
		case "First":
			h = h.turn(2)
		case "Second":
			h = h.turn(4)
		case "Third":
			h = h.turn(8)
		case "Fourth":
			h = h.turn(16)
		case "Fifth":
			h = h.turn(32)
		}
		if h.turned%32 < 31 {
			setInt(st, "stateEx", h.turned*100+h.cards[4])
			htmltext = s.HTMLText("30845-12.htm")
		} else if h.turned%32 == 31 {
			switch h.score() {
			case 40:
				giveReward(s, p, ziggoGem, 43)
				giveReward(s, p, scrollS, 3)
				giveReward(s, p, scrollA, 1)
				htmltext = s.HTMLText("30845-13.htm")
			case 30:
				giveReward(s, p, scrollS, 2)
				giveReward(s, p, scrollC, 2)
				htmltext = s.HTMLText("30845-14.htm")
			case 21, 12:
				giveReward(s, p, scrollA, 1)
				giveReward(s, p, scrollB, 2)
				giveReward(s, p, scrollD, 1)
				htmltext = s.HTMLText("30845-15.htm")
			case 20:
				giveReward(s, p, scrollC, 2)
				htmltext = s.HTMLText("30845-16.htm")
			case 11:
				giveReward(s, p, scrollC, 1)
				htmltext = s.HTMLText("30845-17.htm")
			case 10:
				giveReward(s, p, armorScrollD, 2)
				htmltext = s.HTMLText("30845-18.htm")
			case 0:
				htmltext = s.HTMLText("30845-19.htm")
			}
			setInt(st, "state", 0)
			setInt(st, "stateEx", 0)
		}
		htmltext = h.fill(htmltext)
	case strings.EqualFold(e.Name, "30845-20.htm"):
		if p.ItemCount(redGem) < 50 {
			htmltext = "30845-21.htm"
		}
	}
	return htmltext
}

// deal draws one card of a new hand: a draw of 1 to 70 folded into 1 to 14.
func deal(s *script.Script) int32 {
	n := int32(s.Rnd(70)) + 1
	switch {
	case n >= 57:
		return n - 56
	case n >= 43:
		return n - 42
	case n >= 29:
		return n - 28
	case n >= 15:
		return n - 14
	}
	return n
}

// turn returns the hand with the card whose bit is mask/2 turned over, if it
// is face down.
func (h hand) turn(mask int32) hand {
	if h.turned%mask < mask/2 {
		return hand{cards: h.cards, turned: h.turned + mask/2}
	}
	return h
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 61 {
			htmltext = "30845-02.htm"
		} else {
			htmltext = "30845-01.htm"
		}
	case questlog.StatusStarted:
		state := st.GetInt("state")
		stateEx := st.GetInt("stateEx")
		switch {
		case state == 0 && stateEx == 0:
			if e.Player.ItemCount(redGem) < 50 {
				htmltext = "30845-04.htm"
			} else {
				htmltext = "30845-05.htm"
			}
		case state != 0 && stateEx != 0:
			htmltext = readHand(state, stateEx).fill(s.HTMLText("30845-11a.htm"))
		}
	}
	return htmltext
}

// onMyDying drops a red gem to a party member who has the quest.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	for _, d := range drops() {
		if d.npc == e.NPC.NpcID() {
			s.DropItems(st.Player(), redGem, 1, 0, d.chance)
			return
		}
	}
}

// giveReward gives count of the item: a stackable one at once, any other one
// by one.
func giveReward(s *script.Script, p *script.Player, item, count int32) {
	if s.ItemStackable(item) {
		s.GiveItems(p, item, count)
		return
	}
	for range count {
		s.GiveItems(p, item, 1)
	}
}

// setInt sets the quest variable key to the number n.
func setInt(st *script.QuestState, key string, n int32) {
	st.Set(key, strconv.Itoa(int(n)))
}

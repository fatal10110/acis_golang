// Package q663 is the quest Seductive Whispers: Wilbert, for a character of
// level 50 or more, trades fifty spirit beads for a card game. The player
// draws a card against his, wins a round by a pair or loses it, and a run of
// won rounds pays a prize that grows with its length; a single bead buys a
// smaller game for adena.
package q663

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q663_SeductiveWhispers"

// NPC.
const wilbert = 30846

// Quest item.
const spiritBead = 8766

// Rewards.
const (
	adena          = 57
	enchantWeaponA = 729
	enchantArmorA  = 730
	enchantWeaponB = 947
	enchantArmorB  = 948
	enchantWeaponC = 951
	enchantWeaponD = 955
)

// recipes are the prizes of the fifth round, one of them at random.
func recipes() []int32 {
	return []int32{2353, 4963, 4967, 5000, 5001, 5002, 5004, 5005, 5006, 5007}
}

// blades are the prizes of the sixth round, one of them at random.
func blades() []int32 {
	return []int32{2115, 4104, 4108, 4114, 4115, 4116, 4118, 4119, 4120, 4121}
}

// drop is a monster that drops a spirit bead, with its chance out of a million.
type drop struct{ npc, chance int32 }

// drops lists the monsters that drop spirit beads.
func drops() []drop {
	return []drop{
		{20674, 807000},
		{20678, 372000},
		{20954, 460000},
		{20955, 537000},
		{20956, 540000},
		{20957, 565000},
		{20958, 425000},
		{20959, 682000},
		{20960, 372000},
		{20961, 547000},
		{20962, 522000},
		{20963, 498000},
		{20974, 1000000},
		{20975, 975000},
		{20976, 825000},
		{20996, 385000},
		{20997, 342000},
		{20998, 377000},
		{20999, 450000},
		{21000, 395000},
		{21001, 535000},
		{21002, 472000},
		{21006, 502000},
		{21007, 540000},
		{21008, 692000},
		{21009, 740000},
		{21010, 595000},
	}
}

// New returns the quest.
func New() script.Script {
	monsters := make([]int32, 0, len(drops()))
	for _, d := range drops() {
		monsters = append(monsters, d.npc)
	}
	return script.Script{
		Title:   "Seductive Whispers",
		QuestID: 663,
		Items:   []int32{spiritBead},
		Bind: script.Bindings{
			script.EventQuestStart: {wilbert},
			script.EventTalked:     {wilbert},
			script.EventMyDying:    monsters,
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// card is the text of the face of card n; 0 is the back.
func card(n int32) string {
	switch n {
	case 0:
		return script.NpcString(66300)
	case 11:
		return script.NpcString(66311)
	case 12:
		return script.NpcString(66312)
	case 13:
		return script.NpcString(66313)
	case 14:
		return script.NpcString(66314)
	case 15:
		return script.NpcString(66315)
	case 21:
		return script.NpcString(66321)
	case 22:
		return script.NpcString(66322)
	case 23:
		return script.NpcString(66323)
	case 24:
		return script.NpcString(66324)
	case 25:
		return script.NpcString(66325)
	}
	// Only the cards of the game are asked for; the text of no card is no text.
	return script.NpcString(0)
}

// html is the page with the card of the player (index) and the card drawn
// (param3) shown, and the player's name.
func html(s *script.Script, page string, index, param3 int32, name string) string {
	text := strings.ReplaceAll(s.HTMLText(page), "%card1pic%", card(index))
	text = strings.ReplaceAll(text, "%card2pic%", card(param3))
	return strings.ReplaceAll(text, "%name%", name)
}

// setInt sets the quest variable key to the number n.
func setInt(st *script.QuestState, key string, n int32) {
	st.Set(key, strconv.Itoa(int(n)))
}

// payTopPrize gives the prize of eight won rounds.
func payTopPrize(s *script.Script, p *script.Player) {
	s.RewardItems(p, adena, 2384000)
	s.RewardItems(p, enchantWeaponA, 1)
	s.RewardItems(p, enchantArmorA, 1)
	s.RewardItems(p, enchantArmorA, 1)
}

// onAdvEvent runs the dialog: it starts and ends the quest, buys a game with
// beads, plays its rounds and pays the prize the player takes.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	state := st.GetInt("state")
	switch {
	case strings.EqualFold(e.Name, "30846-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		setInt(st, "state", 1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30846-09.htm") && state%10 <= 4:
		if state/10 < 1 {
			if p.ItemCount(spiritBead) >= 50 {
				s.TakeItems(p, spiritBead, 50)
				setInt(st, "state", 5)
			} else {
				htmltext = "30846-10.htm"
			}
		} else {
			setInt(st, "state", state/10*10+5)
			setInt(st, "stateEx", 0)
			htmltext = "30846-09a.htm"
		}
	case strings.EqualFold(e.Name, "30846-14.htm") && state%10 == 5 && state/1000 == 0:
		i0 := st.GetInt("stateEx")
		i1 := i0 % 10
		i2 := (i0 - i1) / 10
		param1 := int32(s.Rnd(2)) + 1
		param2 := int32(s.Rnd(5)) + 1
		i5 := state / 10
		param3 := param1*10 + param2
		var won bool
		if param1 == i2 {
			i3 := param2 + i1
			won = i3%5 == 0 && i3 != 10
		} else {
			won = param2 == 5 || i1 == 5
		}
		switch {
		case won && state%100/10 >= 7:
			setInt(st, "state", 4)
			payTopPrize(s, p)
			htmltext = html(s, "30846-14.htm", i0, param3, p.Name())
		case won:
			setInt(st, "state", state/10*10+7)
			htmltext = strings.ReplaceAll(html(s, "30846-13.htm", i0, param3, p.Name()), "%wincount%", strconv.Itoa(int(i5+1)))
		default:
			setInt(st, "state", state/10*10+6)
			setInt(st, "stateEx", param3)
			htmltext = html(s, "30846-12.htm", i0, param3, p.Name())
		}
	case strings.EqualFold(e.Name, "30846-19.htm") && state%10 == 6 && state/1000 == 0:
		i0 := st.GetInt("stateEx")
		i1 := i0 % 10
		i2 := (i0 - i1) / 10
		param1 := int32(s.Rnd(2)) + 1
		param2 := int32(s.Rnd(5)) + 1
		param3 := param1*10 + param2
		var lost bool
		if param1 == i2 {
			i3 := param1 + i1
			lost = i3%5 == 0 && i3 != 10
		} else {
			lost = param2 == 5 || i1 == 5
		}
		if lost {
			setInt(st, "state", 1)
			if param1 == i2 {
				setInt(st, "stateEx", 0)
			}
			htmltext = html(s, "30846-19.htm", i0, param3, p.Name())
		} else {
			setInt(st, "state", state/10*10+5)
			setInt(st, "stateEx", param3)
			htmltext = html(s, "30846-18.htm", i0, param3, p.Name())
		}
	case strings.EqualFold(e.Name, "30846-20.htm") && state%10 == 7 && state/1000 == 0:
		setInt(st, "state", (state/10+1)*10+4)
		setInt(st, "stateEx", 0)
	case strings.EqualFold(e.Name, "30846-21.htm") && state%10 == 7 && state/1000 == 0:
		switch state / 10 {
		case 0:
			s.RewardItems(p, adena, 40000)
		case 1:
			s.RewardItems(p, adena, 80000)
		case 2:
			s.RewardItems(p, adena, 110000)
			s.RewardItems(p, enchantWeaponD, 1)
		case 3:
			s.RewardItems(p, adena, 199000)
			s.RewardItems(p, enchantWeaponC, 1)
		case 4:
			s.RewardItems(p, adena, 388000)
			r := recipes()
			s.RewardItems(p, r[s.Rnd(len(r))], 1)
		case 5:
			s.RewardItems(p, adena, 675000)
			b := blades()
			s.RewardItems(p, b[s.Rnd(len(b))], 1)
		case 6:
			s.RewardItems(p, adena, 1284000)
			s.RewardItems(p, enchantWeaponB, 1)
			s.RewardItems(p, enchantArmorB, 1)
			s.RewardItems(p, enchantWeaponB, 1)
			s.RewardItems(p, enchantArmorB, 1)
		}
		setInt(st, "state", 1)
		setInt(st, "stateEx", 0)
	case strings.EqualFold(e.Name, "30846-22.htm") && state%10 == 1:
		if p.HasItem(spiritBead) {
			setInt(st, "state", 1005)
			s.TakeItems(p, spiritBead, 1)
		} else {
			htmltext = "30846-22a.htm"
		}
	case strings.EqualFold(e.Name, "30846-25.htm") && state == 1005:
		i0 := st.GetInt("stateEx")
		i1 := i0 % 10
		i2 := (i0 - i1) / 10
		param1 := int32(s.Rnd(2)) + 1
		param2 := int32(s.Rnd(5)) + 1
		param3 := param1*10 + param2
		var won bool
		if param1 == i2 {
			i3 := param2 + i1
			won = i3%5 == 0 && i3 != 10
		} else {
			won = param2 == 5 || i2 == 5
		}
		if won {
			setInt(st, "state", 1)
			setInt(st, "stateEx", 0)
			s.RewardItems(p, adena, 800)
			htmltext = strings.ReplaceAll(html(s, "30846-25.htm", i0, param3, p.Name()), "%card1%", strconv.Itoa(int(i1)))
		} else {
			setInt(st, "state", 1006)
			setInt(st, "stateEx", param3)
			htmltext = html(s, "30846-24.htm", i0, param3, p.Name())
		}
	case strings.EqualFold(e.Name, "30846-29.htm") && state == 1006:
		i0 := st.GetInt("stateEx")
		i1 := i0 % 10
		i2 := (i0 - i1) / 10
		param1 := int32(s.Rnd(2)) + 1
		param2 := int32(s.Rnd(5)) + 1
		param3 := param1*10 + param2
		switch {
		case param1 == i2 && (param2+i1)%5 == 0 && param2+i1 != 10:
			setInt(st, "state", 1)
			setInt(st, "stateEx", 0)
			s.RewardItems(p, adena, 800)
			htmltext = strings.ReplaceAll(html(s, "30846-29.htm", i0, param3, p.Name()), "%card1%", strconv.Itoa(int(i1)))
		case param1 != i2 && (param2 == 5 || i1 == 5):
			setInt(st, "state", 1)
			setInt(st, "stateEx", 0)
			htmltext = html(s, "30846-29.htm", i0, param3, p.Name())
		default:
			setInt(st, "state", 1005)
			setInt(st, "stateEx", param3)
			htmltext = html(s, "30846-28.htm", i0, param3, p.Name())
		}
	case strings.EqualFold(e.Name, "30846-30.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	}
	return htmltext
}

// onTalk shows the page of the player's state in the game.
func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 50 {
			htmltext = "30846-02.htm"
		} else {
			htmltext = "30846-01.htm"
		}
	case questlog.StatusStarted:
		state := st.GetInt("state")
		switch {
		case state < 4:
			if p.HasItem(spiritBead) {
				htmltext = "30846-05.htm"
			} else {
				htmltext = "30846-04.htm"
			}
		case state%10 == 4:
			htmltext = "30846-05a.htm"
		case state%10 == 5:
			htmltext = "30846-11.htm"
		case state%10 == 6:
			htmltext = "30846-15.htm"
		case state%10 == 7:
			if state%100/10 >= 7 {
				payTopPrize(s, p)
				htmltext = "30846-17.htm"
			} else {
				htmltext = strings.ReplaceAll(s.HTMLText("30846-16.htm"), "%wincount%", strconv.Itoa(int(state/10+1)))
			}
		case state == 1005:
			htmltext = "30846-23.htm"
		case state == 1006:
			htmltext = "30846-26.htm"
		}
	}
	return htmltext
}

// onMyDying drops a spirit bead to a party member who has the quest.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	for _, d := range drops() {
		if d.npc == e.NPC.NpcID() {
			s.DropItems(st.Player(), spiritBead, 1, 0, d.chance)
			return
		}
	}
}

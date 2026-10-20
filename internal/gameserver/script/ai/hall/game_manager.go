package hall

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// WateringGame is what the Rainbow Springs hot springs game managers
// share: the circle they run as and whether they run the word game. Each
// manager script sets them as it is built, so the one built last decides
// for every manager.
type WateringGame struct {
	circle     script.IntCell
	gameMaster script.IntCell
}

// NewWateringGame returns the managers' shared values, circle 0 and no
// word game, as no manager has set them yet.
func NewWateringGame() *WateringGame { return &WateringGame{} }

// rainbowSprings is the siegable hall whose mini game the managers run.
const rainbowSprings = 62

// The pages of the hot springs game managers.
const (
	managerPages    = "data/html/script/siegablehall/RainbowSpringsChateau/"
	wordWrongPage   = "data/html/script/siegablehall/RainbowSpringsChateau/watering_manager002.htm"
	howToPage       = "data/html/script/siegablehall/RainbowSpringsChateau/watering_manager003.htm"
	wordRightPage   = "data/html/script/siegablehall/RainbowSpringsChateau/watering_manager004.htm"
	currentWordPage = "data/html/script/siegablehall/RainbowSpringsChateau/watering_manager005.htm"
)

// quizWord is word i of the managers' word game, i from 0 to 7.
func quizWord(i int32) string {
	return [...]string{
		"BABYDUCK",
		"ALBATROS",
		"PELICAN",
		"KINGFISHER",
		"CYGNUS",
		"TRITON",
		"RAINBOW",
		"SPRING",
	}[i]
}

// letterItems are the letter items of the word game, by letter.
func letterItems() map[string]int32 {
	return map[string]int32{
		"A": 8035, "B": 8036, "C": 8037, "D": 8038, "E": 8039,
		"F": 8040, "G": 8041, "H": 8042, "I": 8043, "K": 8045,
		"L": 8046, "N": 8047, "O": 8048, "P": 8049, "R": 8050,
		"S": 8051, "T": 8052, "U": 8053, "W": 8054, "Y": 8055,
	}
}

// GameManager is AzitWateringGameManager, the base of the Rainbow Springs
// hot springs game managers, which binds no NPC itself. A manager keeps
// the first gourd it sees, the gourds the other managers report and the
// last player it saw, and the party of the first player to talk to it or
// pour on it. It hands a player who brings the letters of the current word
// a prize; on its gourd's breaking it names the winner of the hall's mini
// game; and the hot springs waters poured on it make it cast at its gourd,
// raise an enraged yeti, swap its gourd's hit points with another
// manager's or call the other circles. w is the managers' shared values.
func GameManager(w *WateringGame) script.Script {
	base := defaultnpc.New().Hooks
	players := &script.ListCell[*script.Player]{}
	return script.Script{Dir: "ai/siegeablehall/AzitWateringGameManager", Behavior: true, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			players.Clear()
			for _, slot := range []script.IntSlot{npc.IntAI0, npc.IntAI1, npc.IntAI2, npc.IntAI4, npc.IntQuest1, npc.IntQuest2, npc.IntQuest3, npc.IntQuest4} {
				n.SetInt(slot, 0)
			}
			for _, slot := range []script.CreatureSlot{npc.CreatureAI0, npc.CreatureAI1, npc.CreatureAI2, npc.CreatureAI3, npc.CreatureAI4, npc.CreatureQuest0, npc.CreatureQuest4} {
				n.SetSlot(slot, nil)
			}
			n.SetInt(npc.IntQuest0, -1)
			s.StartTimer("1004", n, nil, 6*10000*time.Millisecond)
			s.StartTimer("1005", n, nil, 45000*time.Millisecond)
			s.StartTimer("1001", n, nil, 10000*time.Millisecond)
		},
		OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
			joinParty(players, e.NPC, e.Player)
			page, _ := s.Page(managerPages + strconv.Itoa(int(e.NPC.NpcID())) + "/watering_manager001.htm")
			return page
		},
		OnEvent: func(s *script.Script, e script.Event) string {
			n := e.NPC
			word := n.Int(npc.IntQuest0)
			var html string
			switch {
			case strings.EqualFold(e.Name, "giveWord"):
				if word == -1 {
					n.Say(script.NpcString(1010628))
				} else if word >= 0 && word <= 7 {
					if checkWordAndGiveItems(s, n, e.Player, quizWord(word)) {
						html, _ = s.Page(wordRightPage)
					} else {
						html, _ = s.Page(wordWrongPage)
					}
				}
			case strings.EqualFold(e.Name, "showCurrentWord"):
				reply, _ := s.Page(currentWordPage)
				if word == -1 {
					reply = strings.ReplaceAll(reply, "%quizstring%", script.NpcString(1010635))
				} else if word >= 0 && word <= 7 {
					reply = strings.ReplaceAll(reply, "%quizstring%", quizWord(word))
				}
				html = reply
			case strings.EqualFold(e.Name, "howTo"):
				html, _ = s.Page(howToPage)
			}
			return html
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			circle := int32(w.circle.Get())
			switch {
			case e.EventID == 10002:
				n.SetSlot(npc.CreatureAI0, s.Object(e.Arg1))
			case e.EventID == 10003:
				n.SetSlot(npc.CreatureAI1, s.Object(e.Arg1))
			case e.EventID == 10004:
				n.SetSlot(npc.CreatureAI2, s.Object(e.Arg1))
			case e.EventID == 10005:
				n.SetSlot(npc.CreatureAI3, s.Object(e.Arg1))
			case e.EventID == circle && e.Arg1 == 30000:
				n.SetInt(npc.IntQuest1, 1)
				n.SetInt(npc.IntQuest2, e.Arg2)
			case e.EventID == circle && n.Int(npc.IntQuest1) == 1 && n.Int(npc.IntQuest2) == e.Arg2:
				n.SetInt(npc.IntQuest1, 2)
				n.SetInt(npc.IntQuest3, e.Arg2)
				pour(s, n)
			case e.EventID == circle && e.Arg1 == 20001:
				// The slowing area of the circle is not switched on: the
				// script leaves it out too, and only arms its end.
				s.StartTimer("5001", n, nil, 60000*time.Millisecond)
			case e.EventID == 1 && e.Arg1 == 10000:
				n.SetInt(npc.IntQuest0, e.Arg2)
				n.BroadcastOnScreen(10000, quizWord(e.Arg2))
			case e.EventID == 5 && e.Arg1 == 40000:
				// A gourd broke. The script's later answer to the same
				// event, which would set IntQuest4, is never reached, so
				// the winner is the player who last poured nectar.
				nameWinner(s, n, players)
			case e.Arg1 == 20002:
				// The end of the slowing area, left out as its start is.
			case e.EventID == circle && e.Arg1 == 50000:
				n.AddCastDesire(n, level1(5039), 1000000)
			case e.EventID == circle && e.Arg1 == 30003:
				if gourd := n.Slot(npc.CreatureQuest4); gourd != nil {
					n.AddCastDesireHold(gourd, level1(4990), 1000000)
				}
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "5001"):
				// The end of the slowing area, left out as its start is.
			case strings.EqualFold(e.Name, "5002"):
				pour(s, n)
			case strings.EqualFold(e.Name, "5005"):
				s.BroadcastScriptEventEx(n, int32(w.circle.Get()), 40000, 0, 1000)
			case strings.EqualFold(e.Name, "1001"):
				n.LookNeighbor()
				if w.gameMaster.Get() == 1 {
					n.Shout(script.NpcString(1010633))
					s.StartTimer("2001", n, e.Player, 1000*time.Millisecond)
				}
			case strings.EqualFold(e.Name, "9999"):
				n.LookNeighbor()
				s.StartTimer("9999", n, nil, 1000*time.Millisecond)
			case strings.EqualFold(e.Name, "2001"):
				// Only the first circle's manager draws the word: the
				// screen message reaches the other circles anyway.
				if w.circle.Get() != 1 {
					return ""
				}
				i0 := int32(s.Rnd(8))
				n.BroadcastOnScreen(10000, quizWord(i0))
				n.SetInt(npc.IntQuest0, i0)
				s.BroadcastScriptEventEx(n, 1, 10000, i0, 10000)
				s.StartTimer("2001", n, nil, 3*60*1000*time.Millisecond)
			case strings.EqualFold(e.Name, "1004"):
				n.LookNeighbor()
				gourd, _ := n.Slot(npc.CreatureQuest4).(*script.NPC)
				if seen, ok := n.Slot(npc.CreatureAI4).(*script.Player); ok && gourd != nil {
					if leader := seen.PartyLeader(); leader != nil {
						n.Shout(script.NpcString(1010634, leader.Name(), int(gourd.HPRatio()*100)))
					}
				}
				s.StartTimer("1004", n, nil, 3*60*1000*time.Millisecond)
			case strings.EqualFold(e.Name, "1005"):
				n.LookNeighbor()
			}
			return ""
		},
		OnSpelled: func(s *script.Script, e script.Spelled) {
			n := e.NPC
			joinParty(players, n, e.Caster)
			switch e.Skill.ID {
			case 2240:
				if s.Rnd(100) < 90 {
					n.SetInt(npc.IntAI0, 1)
					pour(s, n)
					n.SetInt(npc.IntAI1, e.Caster.ObjectID())
				} else {
					at := script.LocOf(n)
					s.Spawner().CreateOnePrivateEx(n, 35592, script.Loc{X: at.X + 10, Y: at.Y + 10, Z: at.Z}, 0, script.Params{})
				}
			case 2241:
				for i2 := int32(1); i2 < 5; i2++ {
					if i2 != int32(w.circle.Get()) {
						s.BroadcastScriptEventEx(n, i2, 30003, 0, 10000)
					}
				}
			case 2242:
				n.LookNeighbor()
				shoutPartyLeader(n, 1010641)
				swapGourds(s, w, n)
			case 2243:
				n.LookNeighbor()
				shoutPartyLeader(n, 1010640)
				for i2 := int32(1); i2 < 5; i2++ {
					if i2 != int32(w.circle.Get()) {
						s.BroadcastScriptEventEx(n, i2, 20001, 0, 10000)
					}
				}
			}
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			n := e.NPC
			if seen, ok := e.Creature.(*script.NPC); ok && n.Slot(npc.CreatureQuest4) == nil {
				if id := seen.NpcID(); id >= 35588 && id <= 35591 {
					n.SetSlot(npc.CreatureQuest4, seen)
				}
			}
			if p, ok := e.Creature.(*script.Player); ok {
				n.SetSlot(npc.CreatureAI4, p)
			}
		},
	})}
}

// joinParty lists the members of p's party as the manager's players, once
// per manager: the first player in a party to talk to it or pour on it.
func joinParty(players *script.ListCell[*script.Player], n *script.NPC, p *script.Player) {
	if members := p.PartyMembers(); members != nil && n.Int(npc.IntAI2) == 0 {
		players.Add(members...)
		n.SetInt(npc.IntAI2, 1)
	}
}

// pour answers what was poured on the manager, or the step its circle's
// exchange is at: a pending nectar, then a pending mineral water, is cast
// at its gourd; else, on the second word with no hit points to set, the
// gourd's hit points go to the circle the exchange names; else the gourd
// takes the hit points the exchange set, breaking at 1 or less, and the
// circle hears what it had.
func pour(s *script.Script, n *script.NPC) {
	gourd, _ := n.Slot(npc.CreatureQuest4).(*script.NPC)
	switch {
	case n.Int(npc.IntAI0) == 1:
		n.AddCastDesireHold(gourd, level1(4989), 10000000)
		n.SetInt(npc.IntAI0, 0)
	case n.Int(npc.IntAI1) == 1:
		n.AddCastDesireHold(gourd, level1(4990), 10000000)
		n.SetInt(npc.IntAI1, 0)
	case n.Int(npc.IntQuest0) == 1 && n.Int(npc.IntQuest3) == 0:
		s.BroadcastScriptEventEx(n, n.Int(npc.IntQuest2), int32(gourd.HP()), 0, 10000)
	case n.Int(npc.IntQuest1) == 2:
		i1 := int32(gourd.HP())
		gourd.SetHP(float64(n.Int(npc.IntQuest3)))
		if n.Int(npc.IntQuest3) <= 1 {
			gourd.DoDie(n.Slot(npc.CreatureAI4))
		}
		s.BroadcastScriptEventEx(n, n.Int(npc.IntQuest2), 30000, i1, 10000)
		n.SetInt(npc.IntQuest1, 0)
		n.SetInt(npc.IntQuest2, 0)
		n.SetInt(npc.IntQuest3, 0)
	}
}

// shoutPartyLeader shouts the NpcString id with the name of the leader of
// the party of the last player the manager saw, when it is in one.
func shoutPartyLeader(n *script.NPC, id int32) {
	if seen, ok := n.Slot(npc.CreatureAI4).(*script.Player); ok {
		if leader := seen.PartyLeader(); leader != nil {
			n.Shout(script.NpcString(id, leader.Name()))
		}
	}
}

// swapGourds gives the manager's gourd the hit points of the gourd of
// another circle, drawn at random among the circles reported, and that
// gourd the hit points the manager's gourd then has; each breaks at 1 or
// less.
func swapGourds(s *script.Script, w *WateringGame, n *script.NPC) {
	i0 := int32(s.Rnd(4) + 1)
	var i4, i5, i6 int32
	for i2 := int32(1); i2 < 5; i2++ {
		if i2 != int32(w.circle.Get()) {
			switch {
			case i4 == 0:
				i4 = i2
			case i5 == 0:
				i5 = i2
			case i6 == 0:
				i6 = i2
			}
		}
		i7 := s.Rnd(100)
		switch {
		case i7 < 33:
			i0 = i4
		case i7 < 66:
			i0 = i5
		default:
			i0 = i6
		}
	}
	var c0 *script.NPC
	switch i0 {
	case 1:
		c0, _ = n.Slot(npc.CreatureAI0).(*script.NPC)
	case 2:
		c0, _ = n.Slot(npc.CreatureAI1).(*script.NPC)
	case 3:
		c0, _ = n.Slot(npc.CreatureAI2).(*script.NPC)
	case 4:
		c0, _ = n.Slot(npc.CreatureAI3).(*script.NPC)
	}
	gourd, _ := n.Slot(npc.CreatureQuest4).(*script.NPC)
	if c0 == nil || gourd == nil {
		return
	}
	gourd.SetHP(c0.HP())
	c0.SetHP(gourd.HP())
	if gourd.HP() <= 1 {
		gourd.DoDie(n.Slot(npc.CreatureAI4))
	}
	if c0.HP() <= 1 {
		c0.DoDie(n.Slot(npc.CreatureAI4))
	}
}

// nameWinner names the winner of the hall's mini game as the manager's
// gourd broke: the player who last poured nectar on the manager while
// within 1000 of it, else the first of the manager's players; and tells the
// managers within 8000 that the game is over.
func nameWinner(s *script.Script, n *script.NPC, players *script.ListCell[*script.Player]) {
	rsc := s.SiegableHall(rainbowSprings)
	if rsc == nil {
		return
	}
	gourd, _ := n.Slot(npc.CreatureQuest4).(*script.NPC)
	id := n.Int(npc.IntQuest4)
	if id == 0 {
		id = n.Int(npc.IntAI1)
	}
	if c0 := s.Object(id); c0 != nil && n.Distance2D(c0) < 1000 {
		rsc.SetMiniGameWinner(gourd, c0.(*script.Player))
		s.BroadcastScriptEvent(n, 0, 20002, 8000)
		return
	}
	for _, pl := range players.Items() {
		rsc.SetMiniGameWinner(gourd, pl)
		s.BroadcastScriptEvent(n, 0, 20002, 8000)
		return
	}
}

// checkWordAndGiveItems takes the letters of word from p and gives it a
// prize, the words 1 to 6 drawing the cheaper prizes more often; false,
// taking nothing, when p lacks a letter.
func checkWordAndGiveItems(s *script.Script, n *script.NPC, p *script.Player, word string) bool {
	letters := letterItems()
	counts := &script.IntMapCell[int32]{}
	for _, ch := range word {
		id := letters[string(ch)]
		c, _ := counts.Get(id)
		counts.Set(id, c+1)
	}
	entries := counts.Entries()
	for _, entry := range entries {
		if p.ItemCount(entry.Key) < entry.Value {
			return false
		}
	}
	for _, entry := range entries {
		s.TakeItems(p, entry.Key, entry.Value)
	}
	i0 := s.Rnd(100)
	if w := n.Int(npc.IntQuest0); w >= 1 && w <= 6 {
		switch {
		case i0 < 70:
			s.GiveItems(p, 8030, 1)
		case i0 < 80:
			s.GiveItems(p, 8031, 1)
		case i0 < 90:
			s.GiveItems(p, 8032, 1)
		default:
			s.GiveItems(p, 8033, 1)
		}
	} else {
		switch {
		case i0 < 10:
			s.GiveItems(p, 8030, 1)
		case i0 < 40:
			s.GiveItems(p, 8031, 1)
		case i0 < 70:
			s.GiveItems(p, 8032, 1)
		default:
			s.GiveItems(p, 8033, 1)
		}
	}
	return true
}

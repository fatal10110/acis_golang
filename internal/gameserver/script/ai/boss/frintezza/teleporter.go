package frintezza

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// msgInventoryLessThan80Percent refuses an overweight entrant.
const msgInventoryLessThan80Percent = 1118

// Entry items: the leader's Frintezza's Magic Force Field Removal Scroll,
// and the Breaking Arrows and Dewdrops of Destruction every entrant leaves
// behind; and the tomb's boss zone.
const (
	forceFieldRemovalScroll = 8073
	breakingArrow           = 8192
	dewdropOfDestruction    = 8556
	frintezzaTombZID        = 110012
)

// The tomb's makers the teleporter signals.
const (
	makerPatrol  = "godard32_2515_05m1"
	makerWizard1 = "godard32_2515_01m1"
	makerWizard2 = "godard32_2515_02m1"
	makerWizard3 = "godard32_2515_03m1"
	makerWizard4 = "godard32_2515_04m1"
	makerWizard5 = "godard32_2515_08m1"
	makerAlarm1  = "godard32_2515_22m1"
	makerAlarm2  = "godard32_2515_19m1"
	makerAlarm3  = "godard32_2515_20m1"
	makerAlarm4  = "godard32_2515_21m1"
)

// teleLoc is where the i-th party of the channel lands in the tomb.
func teleLoc(i int) script.Loc {
	switch i {
	case 0:
		return script.Loc{X: 173247, Y: -76979, Z: -5104}
	case 1:
		return script.Loc{X: 174985, Y: -75161, Z: -5104}
	case 2:
		return script.Loc{X: 173917, Y: -76109, Z: -5104}
	case 3:
		return script.Loc{X: 174037, Y: -76321, Z: -5104}
	}
	return script.Loc{X: 173249, Y: -75154, Z: -5104}
}

// teleporter is the state every Frintezza teleporter shares: a busy mark
// nothing in the class raises (cleared on a reset), and the members of each
// party it let in, kept per channel slot until a later entry with as many
// parties replaces them.
type teleporter struct {
	busy    *script.IntCell
	parties [5]*script.ListCell[*script.Player]
}

// FrintezzaTeleporter (32011), the way into Frintezza's tomb. Created it
// copies Frintezza's db value into its ai slot 0 (0 with no Frintezza) and
// checks the tomb every minute. "story" shows its lore; "enter" lets in the
// leader of a command channel of four or five parties, every member standing
// within party range: the patrol, wizard 1 to 4 and alarm makers get maker
// event 1001, the leader gives up a Magic Force Field Removal Scroll, and party
// by party every member is granted 30 s of tomb entry, teleported to the
// party's spot and stripped of every Breaking Arrow and Dewdrop of
// Destruction. Frintezza is then told 1 and, while his db
// value stays under 2, the channel is told 35 minutes are left and counted
// down at 30, 25, 20, 15, 10, 5, 4, 3, 2 and 1 minutes, each party told
// through one of its stored members still grouped with another. A minute
// after the last notice, with Frintezza still under 2, it resets the tomb:
// ai slot 0 cleared, players sent out to (150037, -57255, -2976), Frintezza
// and his follower told 0, every maker told 1000 and the doors set. The
// minute check clears the slot and sends players out once Frintezza and
// his follower are both at 0, and sends players out once Frintezza is at 3
// or over. Frintezza's script event arg1 0 clears the slot; arg1 8 sets it
// to 8.
func FrintezzaTeleporter() script.Script {
	base := defaultnpc.New().Hooks
	t := &teleporter{busy: &script.IntCell{}}
	for i := range t.parties {
		t.parties[i] = &script.ListCell[*script.Player]{}
	}
	return script.Script{
		Dir: dir, Behavior: true, NPCs: []int32{32011},
		Bind: script.Bindings{script.EventTalked: {32011}},
		Hooks: base.With(script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				n := e.NPC
				c0 := memoNPC(s, "4")

				if c0 == nil {
					n.SetInt(script.SlotIntAI0, 0)
				} else {
					n.SetInt(script.SlotIntAI0, int32(c0.DBValue()))
				}
				n.SetInt(script.SlotIntAI1, 0)
				n.SetInt(npc.IntQuest0, 0)
				n.SetInt(npc.IntQuest1, 0)
				n.SetInt(npc.IntQuest2, 0)
				n.SetInt(npc.IntQuest3, 0)
				n.SetInt(npc.IntQuest4, 0)

				s.StartTimer("5000", n, nil, 60000*time.Millisecond)

				base.Created(s, e)
			},
			OnFirstTalk: func(*script.Script, script.FirstTalk) string {
				return "frintessa_teleporter001.htm"
			},
			OnEvent: t.event,
			OnTimer: func(s *script.Script, e script.Timer) string {
				n := e.NPC
				switch {
				case strings.EqualFold(e.Name, "2006"):
					c0 := memoNPC(s, "4")
					c2 := memoNPC(s, "5")
					if c0 != nil && c2 != nil && c0.DBValue() >= 2 && c2.DBValue() >= 2 {
						return base.Timer(s, e)
					}

					resetTomb(s)

					s.TeleportPlayersInTerritory(n, script.Loc{X: 150037, Y: -57255, Z: -2976}, 150)
				case strings.EqualFold(e.Name, "2001"):
					c0 := memoNPC(s, "4")
					if c0 != nil && c0.DBValue() < 2 {
						n.SetInt(script.SlotIntAI0, 0)
						s.TeleportPlayersInTerritory(n, script.Loc{X: 150037, Y: -57255, Z: -2976}, 150)
						s.SendScriptEvent(c0, n.ObjectID(), 0, 0)
						t.busy.Set(0)

						resetTomb(s)

						if c1 := memoNPC(s, "5"); c1 != nil {
							s.SendScriptEvent(c1, n.ObjectID(), 0, 0)
						}

						s.StartTimer("5000", n, nil, 30*60*1000*time.Millisecond)
					}
				case strings.EqualFold(e.Name, "5000"):
					c0 := memoNPC(s, "4")
					c2 := memoNPC(s, "5")
					if c0 != nil && c2 != nil {
						if c0.DBValue() == 0 && c2.DBValue() == 0 {
							n.SetInt(script.SlotIntAI0, 0)
							t.busy.Set(0)
							s.TeleportPlayersInTerritory(n, script.Loc{X: 150037, Y: -57255, Z: -2976}, 150)
						} else if c0.DBValue() >= 3 {
							s.TeleportPlayersInTerritory(n, script.Loc{X: 150037, Y: -57255, Z: -2976}, 150)
						}
					}
					s.StartTimer("5000", n, nil, 60*1000*time.Millisecond)
				case strings.EqualFold(e.Name, "4030"):
					if tombAwake(s) {
						return base.Timer(s, e)
					}

					t.notifyRemainingTime(30)
					s.StartTimer("4025", n, nil, 5*60*1000*time.Millisecond)
				case strings.EqualFold(e.Name, "4025"):
					if tombAwake(s) {
						return base.Timer(s, e)
					}

					t.notifyRemainingTime(25)
					s.StartTimer("4020", n, nil, 5*60*1000*time.Millisecond)
				case strings.EqualFold(e.Name, "4020"):
					if tombAwake(s) {
						return base.Timer(s, e)
					}

					t.notifyRemainingTime(20)
					s.StartTimer("4015", n, nil, 5*60*1000*time.Millisecond)
				case strings.EqualFold(e.Name, "4015"):
					if tombAwake(s) {
						return base.Timer(s, e)
					}

					t.notifyRemainingTime(15)
					s.StartTimer("4010", n, nil, 5*60*1000*time.Millisecond)
				case strings.EqualFold(e.Name, "4010"):
					if tombAwake(s) {
						return base.Timer(s, e)
					}

					t.notifyRemainingTime(10)
					s.StartTimer("4005", n, nil, 5*60*1000*time.Millisecond)
				case strings.EqualFold(e.Name, "4005"):
					if tombAwake(s) {
						return base.Timer(s, e)
					}

					t.notifyRemainingTime(5)
					s.StartTimer("4004", n, nil, 60*1000*time.Millisecond)
				case strings.EqualFold(e.Name, "4004"):
					if tombAwake(s) {
						return base.Timer(s, e)
					}

					t.notifyRemainingTime(4)
					s.StartTimer("4003", n, nil, 60*1000*time.Millisecond)
				case strings.EqualFold(e.Name, "4003"):
					if tombAwake(s) {
						return base.Timer(s, e)
					}

					t.notifyRemainingTime(3)
					s.StartTimer("4002", n, nil, 60*1000*time.Millisecond)
				case strings.EqualFold(e.Name, "4002"):
					if tombAwake(s) {
						return base.Timer(s, e)
					}

					t.notifyRemainingTime(2)
					s.StartTimer("4001", n, nil, 60*1000*time.Millisecond)
				case strings.EqualFold(e.Name, "4001"):
					if tombAwake(s) {
						return base.Timer(s, e)
					}

					t.notifyRemainingTime(1)
					s.StartTimer("2001", n, nil, 60*1000*time.Millisecond)
				}
				return base.Timer(s, e)
			},
			OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
				c0 := memoNPC(s, "4")
				if c0 != nil && e.EventID == c0.ObjectID() {
					switch e.Arg1 {
					case 0:
						e.NPC.SetInt(script.SlotIntAI0, 0)
						t.busy.Set(0)
					case 8:
						e.NPC.SetInt(script.SlotIntAI0, 8)
					}
				}
			},
		}),
	}
}

// event answers "story" with the lore page and runs the entry on "enter",
// both ignoring case; any other event answers nothing.
func (t *teleporter) event(s *script.Script, e script.Event) string {
	htmltext := ""

	if strings.EqualFold(e.Name, "story") {
		htmltext = "frintessa_teleporter001a.htm"
	} else if strings.EqualFold(e.Name, "enter") {
		n, player := e.NPC, e.Player
		if player.Overweight() {
			player.SystemMessage(msgInventoryLessThan80Percent)
			return htmltext
		}

		c1 := memoNPC(s, "4")
		c2 := memoNPC(s, "5")
		c3 := memoNPC(s, "6")

		if n.Int(script.SlotIntAI0) != 0 {
			if c1 == nil || c3 == nil {
				htmltext = "frintessa_teleporter002.htm"
			} else {
				htmltext = "frintessa_teleporter007.htm"
			}
			return htmltext
		}

		if c1 == nil || c2 == nil || c3 == nil {
			return "frintessa_teleporter002.htm"
		}

		c0 := player.ChannelLeader()
		if c0 == nil {
			return "frintessa_teleporter003.htm"
		}

		switch {
		case !script.Same(player, c0):
			htmltext = "frintessa_teleporter003.htm"
		case !player.HasItem(forceFieldRemovalScroll):
			htmltext = "frintessa_teleporter004.htm"
		default:
			parties := player.ChannelParties()
			ccSize := len(parties)
			switch {
			case ccSize < 4:
				htmltext = "frintessa_teleporter005.htm"
			case ccSize > 5:
				htmltext = "frintessa_teleporter006.htm"
			default:
				for _, p := range player.ChannelMembers() {
					if !s.InPartyRange(p, n) {
						return "frintessa_teleporter008.htm"
					}
				}

				if t.busy.Get() == 1 {
					t.busy.Set(0)
					htmltext = "frintessa_teleporter007.htm"
				} else {
					t.admit(s, n, player, c1, parties)
				}
			}
		}
	}
	return htmltext
}

// admit lets the leader's channel into the tomb, its parties as listed.
func (t *teleporter) admit(s *script.Script, n *script.NPC, player *script.Player, c1 *script.NPC, parties [][]*script.Player) {
	for _, maker := range []string{
		makerPatrol, makerWizard1, makerWizard2, makerWizard3, makerWizard4,
		makerAlarm1, makerAlarm2, makerAlarm3, makerAlarm4,
	} {
		s.Spawner().SendMakerEvent(maker, "1001", 0, 0)
	}

	s.TakeItems(player, forceFieldRemovalScroll, 1)

	frintezzaTomb := s.Zone(frintezzaTombZID)

	for i, party := range parties {
		t.parties[i].Clear()
		t.parties[i].Add(party...)

		at := teleLoc(i)
		for _, member := range party {
			frintezzaTomb.AllowPlayerEntry(member, 30)

			member.TeleportTo(at, 0)

			s.TakeItems(member, breakingArrow, -1)
			s.TakeItems(member, dewdropOfDestruction, -1)
		}
	}

	n.SetInt(script.SlotIntAI0, 1)

	s.SendScriptEvent(c1, n.ObjectID(), 1, 0)

	if c1.DBValue() >= 2 {
		return
	}

	player.ShowChannelScreenMessage(10000, script.NpcString(1010643, 35))

	s.StartTimer("4030", n, nil, 300000*time.Millisecond)
}

// tombAwake reports whether Frintezza and his follower are both named and
// Frintezza's db value is 2 or over, which ends the countdown.
func tombAwake(s *script.Script) bool {
	c0 := memoNPC(s, "4")
	c2 := memoNPC(s, "5")
	return c0 != nil && c2 != nil && c0.DBValue() >= 2
}

// resetTomb tells the patrol, the five wizard and the four alarm makers
// 1000 and sets the tomb doors with grave pathways 3 and 4 open.
func resetTomb(s *script.Script) {
	for _, maker := range []string{
		makerPatrol, makerWizard1, makerWizard2, makerWizard3, makerWizard4, makerWizard5,
		makerAlarm1, makerAlarm2, makerAlarm3, makerAlarm4,
	} {
		s.Spawner().SendMakerEvent(maker, "1000", 0, 0)
	}

	setTombDoors(s, 0)
}

// notifyRemainingTime shows every stored party that leftMinutes are left,
// for 10 s. A party is reached through the first of its stored members
// still in a party with another of them; a party none of whose stored
// members are still grouped together is told nothing.
func (t *teleporter) notifyRemainingTime(leftMinutes int) {
	text := script.NpcString(1010643, leftMinutes)
	for _, slot := range t.parties {
		if p := groupedMember(slot); p != nil {
			p.ShowPartyScreenMessage(10000, text)
		}
	}
}

// groupedMember returns the first player of slot whose party holds another
// player of slot, nil when there is none.
func groupedMember(slot *script.ListCell[*script.Player]) *script.Player {
	for _, p := range slot.Items() {
		for _, m := range p.PartyMembers() {
			if !script.Same(m, p) && slot.Index(m) >= 0 {
				return p
			}
		}
	}
	return nil
}

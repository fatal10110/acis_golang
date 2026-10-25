package drchaos

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// summonRaid is the Gigantic Golem Doctor Chaos raises.
const summonRaid = 25512

// strangeMachineID is the machine Doctor Chaos works at.
const strangeMachineID = 32032

// Where Doctor Chaos walks, where the golem rises, and where the party and
// Doctor Chaos land.
var (
	posMachine = script.Loc{X: 96320, Y: -110912, Z: -3360}
	posRaid    = script.Loc{X: 94640, Y: -112496, Z: -3360}
	posDoor    = script.Loc{X: 96016, Y: -110736, Z: -3376}
)

// Doctor Chaos's script memory: slotStarted is 1 once the cutscene began,
// slotTalker the player who talked to it, slotMachine the machine it saw
// (itself when it saw none).
const (
	slotStarted = script.SlotIntAI1
	slotTalker  = script.SlotCreature1
	slotMachine = script.SlotCreature2
)

// DoctorChaosNpc is Doctor Chaos (32033), who leads a player to the
// Gigantic Golem. Talked to, it speaks a line, remembers the player and,
// 2 s later, looks around and walks to its machine, which it notes on
// sight. There it wants to strike the machine and, 3 s later, starts its
// cutscene if the player stands within 600: a line, a camera on the
// machine with a social action 2 s later, and 11 s after the line a camera
// on itself and a walk to the door; otherwise it walks home and forgets the
// player. At the door it speaks a last line, sends the player, or the
// player's whole party, to the golem's room, leaves for -113091,-243942,
// -15536 and, 3 s later, raises the golem. Created at z -15536, it raises
// the golem at once. Script event 10029, or the death of a private it
// leads, takes it out of the world.
func DoctorChaosNpc() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: "ai/boss/drchaos", Behavior: true, NPCs: []int32{32033}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(slotStarted, 0)
			n.SetSlot(slotTalker, nil)
			n.SetSlot(slotMachine, nil)

			if script.LocOf(n).Z == -15536 {
				s.Spawner().CreateOnePrivateEx(n, summonRaid, posRaid, 0, script.Params{})
			}

			base.Created(s, e)
		},
		OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
			n := e.NPC
			n.SetSlot(slotTalker, e.Player)
			n.Say(script.NpcString(1010578))
			s.StartTimer("1001", n, e.Player, 2000*time.Millisecond)

			return ""
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n, name := e.NPC, e.Name
			switch {
			case strings.EqualFold(name, "1001"):
				n.LookNeighbor()
				n.AddMoveToDesire(posMachine, 1000)
			case strings.EqualFold(name, "1002"):
				n.SpecialCamera(script.CameraShot{Distance: 1, Yaw: -150, Pitch: 10, Time: 3000, Duration: 20000, Turn: 0, Rise: 0, Widescreen: 0, Unknown: 0})
				n.AddMoveToDesire(posDoor, 1000)
			case strings.EqualFold(name, "2001"):
				if talker, ok := n.Slot(slotTalker).(*script.Player); ok && n.Distance3D(talker) < 600 {
					n.SetInt(slotStarted, 1)
					n.Say(script.NpcString(1010579))
					s.StartTimer("2002", n, nil, 2000*time.Millisecond)
					s.StartTimer("1002", n, nil, 11000*time.Millisecond)
				} else {
					n.AddMoveToDesire(n.SpawnLocation(), 1000)
					n.SetSlot(slotTalker, nil)
				}
			case strings.EqualFold(name, "2002"):
				if n.Slot(slotMachine) == nil {
					n.SetSlot(slotMachine, n)
				}

				if machine, ok := n.Slot(slotMachine).(*script.NPC); ok {
					machine.SpecialCamera(script.CameraShot{Distance: 1, Yaw: -200, Pitch: 15, Time: 10000, Duration: 20000, Turn: 0, Rise: 0, Widescreen: 0, Unknown: 0})
				}
				n.AddSocialDesire(3, 5000, 1000000)
			case strings.EqualFold(name, "3001"):
				s.Spawner().CreateOnePrivateEx(n, summonRaid, posRaid, 0, script.Params{P1: 1000, P2: n.Slot(slotTalker).ObjectID(), P3: 0})
			}

			return base.Timer(s, e)
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if c, ok := e.Creature.(*script.NPC); ok && c.NpcID() == strangeMachineID {
				e.NPC.SetSlot(slotMachine, e.Creature)
			}
		},
		OnMoveToFinished: func(s *script.Script, e script.MoveToFinished) {
			n := e.NPC
			home := n.SpawnLocation()
			x, y := int(e.X), int(e.Y)
			switch {
			case x == home.X && y == home.Y:
				n.RemoveAllDesire()
			case x == posMachine.X && y == posMachine.Y:
				n.AddAttackDesire(n.Slot(slotMachine), 10)
				s.StartTimer("2001", n, nil, 3000*time.Millisecond)
			case x == posDoor.X && y == posDoor.Y && n.Slot(slotTalker) != nil:
				n.Say(script.NpcString(1010580))

				talker, _ := n.Slot(slotTalker).(*script.Player)
				if party := talker.PartyMembers(); party == nil {
					talker.TeleportTo(script.Loc{X: 94832, Y: -112624, Z: -3328}, 0)
				} else {
					for _, member := range party {
						member.TeleportTo(script.Loc{X: 94832, Y: -112624, Z: -3328}, 0)
					}
				}

				s.StartTimer("3001", n, nil, 3000*time.Millisecond)

				n.TeleportTo(script.Loc{X: -113091, Y: -243942, Z: -15536})
			}
		},
		OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
			if e.EventID == 10029 {
				e.NPC.DeleteMe()
			}
		},
		OnPartyDied: func(_ *script.Script, e script.PartyDied) {
			if !script.Same(e.Caller, e.Called) {
				e.Called.DeleteMe()
			}
		},
	})}
}

package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// SaintNinja is SaintNinja: a WarriorPhysicalSpecial of the Monastery of
// Silence that buffs itself when created and, out of a fight, flashes its
// teleport effect every five minutes after the first. Its main form (AI
// parameter IsMainForm 1), on its first hits, may once call its other self
// (AI parameter OtherSelf) beside it against the attacker; any other form
// vanishes when five minutes pass out of a fight. A clan member hit
// farther than 300 from it, out of a fight, brings it to the attacker.
//
// Its memory: IntAI0 is set once it called its other self.
func SaintNinja() script.Script {
	base := PhysicalSpecialHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{21539, 21540, 21524, 21525, 21531, 21658}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, 0)
			addCastByType(n, n, npc.SkillTypeSelfBuff, 1000000)
			if n.IntParam("IsMainForm") == 0 {
				s.StartTimer("2000", n, nil, 60000*5*time.Millisecond)
			}
			s.StartTimer("2001", n, nil, 60000*time.Millisecond)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if s.Rnd(100) < 80 && n.IntParam("IsMainForm") == 1 && n.Int(script.SlotIntAI0) == 0 {
				otherSelf := n.IntParam("OtherSelf")
				at := script.LocOf(n)
				x := at.X + s.Rnd(20)
				y := at.Y + s.Rnd(20)
				s.Spawner().CreateOnePrivateEx(n, otherSelf, script.Loc{X: x, Y: y, Z: at.Z, Heading: 32768}, 0, script.Params{P1: 1000, P2: e.Attacker.ObjectID(), P3: 1})
				n.SetInt(script.SlotIntAI0, 1)
			}
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "2000") {
				if n.Intention() != script.IntentionAttack {
					// A form that vanishes reaches no parent.
					s.Spawner().DecayMe(n)
					return ""
				}
				s.StartTimer("2000", n, nil, 60000*5*time.Millisecond)
			} else if strings.EqualFold(e.Name, "2001") {
				if n.Intention() != script.IntentionAttack {
					addCastByType(n, n, npc.SkillTypeTeleportEffect, 1000000)
				}
				s.StartTimer("2001", n, nil, 60000*5*time.Millisecond)
			}
			return base.Timer(s, e)
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.Intention() != script.IntentionAttack && called.Distance2D(attacker) > 300 {
				called.AbortAll()
				called.TeleportTo(attacker)
				addCastByType(called, attacker, npc.SkillTypeTeleportEffect, 1000000)
				if attacker.Playable() {
					f0 := damageHate(called, float64(e.Damage), defaultnpc.HateRatio(called, attacker))
					called.AddAttackDesire(attacker, float64(int(f0*30)))
				}
			}
		},
	})}
}

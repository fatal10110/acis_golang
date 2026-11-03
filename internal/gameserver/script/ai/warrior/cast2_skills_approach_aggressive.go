package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// dewdropOfDestruction is the item a hall keeper drops once it has heard
// nine of its kind die.
const dewdropOfDestruction = 8556

// The NPCs of Cast2SkillsApproachAggressive that count the deaths of their
// kind: a hall keeper captain counts in IntAI4, a hall keeper guard in
// IntAI3.
const (
	hallKeeperCaptainID = 18329
	hallKeeperGuardID   = 18331
)

// The script event arguments a dying hall keeper captain or guard sends to
// the NPCs around it, and the ones a hall keeper sends naming the creature
// it fights.
const (
	argCaptainDied = 10037
	argGuardDied   = 10038
	argFoughtFirst = 10042
	argFoughtLater = 10043
)

// Cast2SkillsApproachAggressive is WarriorCast2SkillsApproachAggressive:
// a hall keeper of the Frintezza raid. It buffs itself whenever it sees a
// creature and attacks the players and summons it sees in its territory;
// hit, it may strike with its physical special and names the creature it
// fights to the NPCs around it, 40 s after it appeared and 60 s later; a
// cleric's spell may turn it on the caster alone. A hall keeper captain or
// guard tells the NPCs around it of its death, and drops a Dewdrop of
// Destruction once it heard nine of its kind die.
//
// Its memory: IntAI3 counts the guards it heard die, IntAI4 the captains,
// and Creature1 is the creature it last fought.
func Cast2SkillsApproachAggressive() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{18329, 18331, 18332, 18335, 18336}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI1, 0)
			n.SetInt(script.SlotIntAI3, 0)
			n.SetInt(script.SlotIntAI4, 0)
			s.StartTimer("1009", n, nil, 40000*time.Millisecond)
		},
		OnNoDesire: func(*script.Script, script.NoDesire) {},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			addCastByType(n, n, npc.SkillTypeSelfBuff, 1000000)
			if !c.Playable() {
				return
			}
			if n.InMyTerritory() {
				n.AddAttackDesire(c, 200)
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if s.Rnd(100) < 20 && e.Caster.InClassGroup("@cleric_group") {
				e.NPC.RemoveAllDesire()
				e.NPC.AddAttackDesire(e.Caster, 200000)
			}
		},
		OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if e.Arg1 == argCaptainDied && n.NpcID() == hallKeeperCaptainID {
				n.SetInt(script.SlotIntAI4, n.Int(script.SlotIntAI4)+1)
			} else if e.Arg1 == argGuardDied && n.NpcID() == hallKeeperGuardID {
				n.SetInt(script.SlotIntAI3, n.Int(script.SlotIntAI3)+1)
			}
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() {
				damage := e.Damage
				if damage == 0 {
					damage = 1
				}
				called.AddAttackDesire(attacker, ((1.0*float64(damage))/float64(called.Level()+7))*1000)
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "1008") {
				if c1 := n.Slot(script.SlotCreature1); c1 != nil {
					s.BroadcastScriptEventEx(n, 0, argFoughtLater, c1.ObjectID(), 8000)
				}
			} else if strings.EqualFold(e.Name, "1009") {
				if c1 := n.Slot(script.SlotCreature1); c1 != nil {
					s.BroadcastScriptEventEx(n, 0, argFoughtFirst, c1.ObjectID(), 8000)
				}
				s.StartTimer("1008", n, nil, 60000*time.Millisecond)
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if s.Rnd(100) < 33 {
				addCastByType(n, e.Attacker, npc.SkillTypePhysicalSpecial, 1000000)
			}
			if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil {
				n.SetSlot(script.SlotCreature1, topDesireTarget)
			}
			base.Attacked(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			if n.Int(script.SlotIntAI4) == 9 {
				n.DropItem(e.Killer, dewdropOfDestruction, 1)
			} else if n.NpcID() == hallKeeperCaptainID {
				s.BroadcastScriptEventEx(n, 0, argCaptainDied, 0, 5000)
			}
			if n.Int(script.SlotIntAI3) == 9 {
				n.DropItem(e.Killer, dewdropOfDestruction, 1)
			} else if n.NpcID() == hallKeeperGuardID {
				s.BroadcastScriptEventEx(n, 0, argGuardDied, 0, 5000)
			}
		},
	})}
}

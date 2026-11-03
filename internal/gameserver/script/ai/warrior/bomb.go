package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Bomb is WarriorBomb: a Warrior that, hit from closer than 200, wants to
// blow up around itself with its self range magic, and dies when that
// blast lands.
func Bomb() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{21666, 21689, 21712, 21735, 21758, 21781}, Hooks: bombHooks()}
}

// bombHooks are WarriorBomb's hooks, which its children derive theirs
// from.
func bombHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.Distance2D(e.Attacker) < 200 {
				castByType(n, n, npc.SkillTypeSelfRangeDDMagic)
			}
			base.Attacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if selfRangeDDMagic, ok := n.SkillByType(npc.SkillTypeSelfRangeDDMagic); ok && e.Skill.ID == selfRangeDDMagic.ID {
				if e.Success {
					n.DoDie(n)
				}
				n.AddCastDesire(n, selfRangeDDMagic, 1000000)
			}
			base.UseSkillFinished(s, e)
		},
	}).Named("WarriorBomb")
}

// BombAggressive is WarriorBombAggressive: a WarriorBomb that attacks the
// players and summons it sees.
func BombAggressive() script.Script {
	base := bombHooks()
	hooks := base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	})
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{18333, 22128}, Hooks: hooks}
}

// isSkillOfType reports whether ref is the skill n's template lists under
// t; a template that lists none matches no skill.
func isSkillOfType(n *script.NPC, ref skill.Ref, t npc.SkillType) bool {
	listed, ok := n.SkillByType(t)
	return ok && ref == listed
}

// TeleportBombAggressive is WarriorTeleportBombAggressive: a WarriorBomb
// that jumps onto the first player or summon it sees, or that attacks its
// clan, flashing its teleport effect, and five seconds after that effect
// casts one of its three self range blasts, dying when the second or the
// third lands. Until then, idle for 10 seconds or more, it jumps back to a
// point its spawn gives.
//
// Its memory: IntAI0 is the tick of its creation or its last jump back,
// IntAI1 is set once it jumped onto a creature.
func TeleportBombAggressive() script.Script {
	base := bombHooks()
	hooks := base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, s.CurrentTick())
			n.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnNoDesire: func(s *script.Script, e script.NoDesire) {
			n := e.NPC
			if i0 := s.ElapsedTicks(n.Int(script.SlotIntAI0)); i0 >= 10 && n.Int(script.SlotIntAI1) == 0 {
				n.TeleportTo(s.Spawner().DrawSpawnLocation(n))
				castByType(n, n, npc.SkillTypeTeleportEffect)
				n.SetInt(script.SlotIntAI0, s.CurrentTick())
			}
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if n.Int(script.SlotIntAI1) == 0 && c.Playable() {
				n.SetInt(script.SlotIntAI1, 1)
				n.TeleportTo(script.LocOf(c))
				castByType(n, n, npc.SkillTypeTeleportEffect)
			}
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.Int(script.SlotIntAI1) == 0 && attacker.Playable() {
				called.SetInt(script.SlotIntAI1, 1)
				called.TeleportTo(script.LocOf(attacker))
				castByType(called, called, npc.SkillTypeTeleportEffect)
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n, k := e.NPC, e.Skill
			switch {
			case isSkillOfType(n, k, npc.SkillTypeTeleportEffect) && n.Int(script.SlotIntAI1) == 1:
				s.StartTimer("1001", n, nil, 5000*time.Millisecond)
			case isSkillOfType(n, k, npc.SkillTypeSelfRangeDDMagic2) && e.Success:
				n.DoDie(n)
			case isSkillOfType(n, k, npc.SkillTypeSelfRangeDDMagic3) && e.Success:
				n.DoDie(n)
			}
			base.UseSkillFinished(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1001") {
				n := e.NPC
				switch i0 := s.Rnd(100); {
				case i0 < 33:
					castByType(n, n, npc.SkillTypeSelfRangeDDMagic)
				case i0 < 66:
					castByType(n, n, npc.SkillTypeSelfRangeDDMagic2)
				default:
					castByType(n, n, npc.SkillTypeSelfRangeDDMagic3)
				}
			}
			return ""
		},
	})
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{22133}, Hooks: hooks}
}

package raid

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// PrivateHealer is RaidPrivateHealer: a raid private that heals its party.
// A hit on its party has it want its heal on the member hit at 100, at
// 1000000 one time in 3 when that member is under half its HP, and its
// curse at 1000000 on a hitter more than 8 levels above it. Each minute it
// may want its self range buff, and once it healed it runs from the last
// playable that hit it when that one stands within 200. It answers no hit
// on itself and no spell it sees, beyond leaving a peace zone.
func PrivateHealer() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidPrivate/RaidPrivateHealer", Behavior: true, NPCs: privateHealerIDs(), Hooks: privateHealerHooks()}
}

// privateHealerIDs are RaidPrivateHealer's own NPCs.
func privateHealerIDs() []int32 {
	return []int32{
		25008, 25017, 25021, 25030, 25045, 25052, 25074, 25080, 25100, 25135, 25144, 25160, 25167, 25174,
		25183, 25203, 25227, 25236, 25239, 25278, 25287, 25292, 25313, 25321, 25340, 25343, 25351, 25359,
		25368, 25377, 25396, 25400, 25417, 25422, 25432, 25449, 25452, 25455, 25458, 25465, 25480, 25483,
		29031, 29039, 29043, 25525, 29057, 29064,
	}
}

// leavePeace has n, inside a peace zone, stop, jump back to its spawn and
// drop every attack desire.
func leavePeace(n *script.NPC) {
	if n.InsideZone(script.ZonePeace) {
		n.AbortAll()
		n.InstantTeleportTo(n.SpawnLocation())
		n.RemoveAllAttackDesire()
	}
}

// fleeNear has n run from the creature c_ai0 holds, at 100000000 as far as
// its drift range, when that one is not n itself and stands within 200.
func fleeNear(n *script.NPC) {
	if c0 := n.Slot(script.SlotCreature0); c0 != nil && !script.Same(c0, n) && n.Distance2D(c0) < 200 {
		n.AddFleeDesire(c0, n.MaxDriftRange(), 100000000)
	}
}

// privateHealerHooks are RaidPrivateHealer's hooks: c_ai0 holds the last
// playable that hit it, itself until one does.
func privateHealerHooks() script.Hooks {
	base := PrivateStandardHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetSlot(script.SlotCreature0, e.NPC)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1001") {
				if s.Rnd(3) < 1 {
					castByType(e.NPC, e.NPC, npc.SkillTypeSelfRangeBuffA)
				}
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			if e.Attacker.Playable() {
				e.NPC.SetSlot(script.SlotCreature0, e.Attacker)
			}
			leavePeace(e.NPC)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called := e.Called
			if e.Target.Level() > called.Level()+8 {
				called.AddCastDesire(e.Target, raidCurse(), 1000000)
			}
			heal, ok := called.SkillByType(npc.SkillTypeHealMagicA)
			if e.Caller.HPRatio() < 0.5 && s.Rnd(3) < 1 && ok {
				called.AddCastDesire(e.Caller, heal, 1000000)
			}
			if ok {
				called.AddCastDesire(e.Caller, heal, 100)
			}
			leavePeace(called)
		},
		// A spell it sees reaches none of the hooks it derives from.
		OnSeeSpell: func(*script.Script, script.SeeSpell) {},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			if heal, ok := e.NPC.SkillByType(npc.SkillTypeHealMagicA); ok && e.Skill.ID == heal.ID {
				fleeNear(e.NPC)
			}
		},
	}).Named("RaidPrivateHealer")
}

// PrivateWizard is RaidPrivateWizard: a raid private that casts. A
// playable that hits its party or casts in its sight may draw its magic
// strike (one time in 30, 300 for a hitter 8 or more levels above it) and,
// when not yet under it, its debuff (one time in 75, 750 for that higher
// hitter beside the 75 roll); a hit on itself does so only once its master
// is dead. Each minute it runs from the last creature that hit it when
// that one stands within 200.
func PrivateWizard() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidPrivate/RaidPrivateWizard", Behavior: true, NPCs: privateWizardIDs(), Hooks: privateWizardHooks()}
}

// privateWizardIDs are RaidPrivateWizard's own NPCs.
func privateWizardIDs() []int32 {
	return []int32{
		25002, 25033, 25061, 25071, 25083, 25090, 25096, 25110, 25113, 25120, 25129, 25138, 25156, 25180,
		25190, 25200, 25209, 25215, 25221, 25224, 25242, 25250, 25253, 25264, 25274, 25284, 25291, 25297,
		25303, 25320, 25323, 25329, 25330, 25331, 25345, 25347, 25350, 25399, 25403, 25416, 25421, 25436,
		25440, 25451, 25454, 25471, 25479, 25488, 29034, 29038, 29044, 25508, 29063, 25515,
	}
}

// privateWizardHooks are RaidPrivateWizard's hooks: c_ai0 holds the last
// creature that hit it, itself until one does.
func privateWizardHooks() script.Hooks {
	base := PrivateStandardHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetSlot(script.SlotCreature0, e.NPC)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1001") {
				fleeNear(e.NPC)
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			n.SetSlot(script.SlotCreature0, attacker)
			if n.HasMaster() && n.Master().Dead() {
				if attacker.Playable() && s.Rnd(30) < 1 {
					castByType(n, attacker, npc.SkillTypeDDMagicA)
				}
				debuff, _ := n.SkillByType(npc.SkillTypeDebuffA)
				if s.AbnormalLevel(attacker, debuff) == -1 && s.Rnd(75) < 1 {
					castByType(n, attacker, npc.SkillTypeDebuffA)
				}
			}
			leavePeace(n)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			debuff, _ := called.SkillByType(npc.SkillTypeDebuffA)
			if target.Level() >= called.Level()+8 {
				if target.Playable() && s.Rnd(300) < 1 {
					castByType(called, target, npc.SkillTypeDDMagicA)
				}
				if s.AbnormalLevel(target, debuff) == -1 && s.Rnd(750) < 1 {
					castByType(called, target, npc.SkillTypeDebuffA)
				}
			} else if target.Playable() && s.Rnd(30) < 1 {
				castByType(called, target, npc.SkillTypeDDMagicA)
			}
			if s.AbnormalLevel(target, debuff) == -1 && s.Rnd(75) < 1 {
				castByType(called, target, npc.SkillTypeDebuffA)
			}
			base.PartyAttacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			if s.Rnd(30) < 1 {
				castByType(n, caster, npc.SkillTypeDDMagicA)
			}
			debuff, _ := n.SkillByType(npc.SkillTypeDebuffA)
			if s.AbnormalLevel(caster, debuff) == -1 && s.Rnd(75) < 1 {
				castByType(n, caster, npc.SkillTypeDebuffA)
			}
			base.SeeSpell(s, e)
		},
	}).Named("RaidPrivateWizard")
}

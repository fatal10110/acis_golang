package wizard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// The passes a gatekeeper lets by, and the reach of its spells from where it
// stands unless its SkillRange says otherwise.
const (
	drainPass1      = 8064
	drainPass2      = 8065
	drainPass3      = 8067
	drainSkillRange = 500
)

// drainSpell makes n want to cast its DD magic at c, weight 1000000, when
// it has the MP and HP the cast takes. Otherwise n gives up its spells
// (int slot 0 set to 1) and, when it may attack c on its own, wants to
// attack c, weight 1000. No c casts and attacks nothing.
func drainSpell(n *script.NPC, c script.Creature) {
	ddMagic, _ := n.SkillByType(npc.SkillTypeDDMagic)
	if n.MeetsHPMPConditions(c, ddMagic) {
		n.AddCastDesire(c, ddMagic, 1000000)
		return
	}
	n.SetInt(script.SlotIntAI0, 1)
	if n.CanAutoAttack(c) {
		n.AddAttackDesire(c, 1000)
	}
}

// drainSummonKilled turns n on the owner of c, a summon it has just
// killed: its DD magic, else an attack, and 200 on its hate list. A summon
// whose owner has left turns it on no one.
func drainSummonKilled(n *script.NPC, c script.Creature) {
	master := script.ActingPlayer(c)
	if master == nil {
		return
	}
	drainSpell(n, master)
	n.HateList().Add(master, 200)
}

// DoormanOfDrain is DoormanOfDrain: a gatekeeper, a Wizard that keeps to
// its spawn point, standing still once there. It strikes the player it
// sees inside its territory once settled, and puts them on its hate list,
// unless they carry one of its passes: then it may heal them and drops
// them from its desires and hate list. Its hit and its clan's hits turn
// its DD magic on what it fights; a summon it kills turns it on the
// summon's owner. A cast ends with the target within its SkillRange
// striking again, or beyond it dropping every desire. A skill with aggro,
// power or offense makes it want to attack the caster.
func DoormanOfDrain() script.Script {
	base := WizardHooks()
	return script.Script{Dir: "ai/individual/Monster/WizardBase/Wizard", Behavior: true, NPCs: []int32{18343, 22136}, Hooks: base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.AddMoveToDesire(e.NPC.SpawnLocation(), 30)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n := e.NPC
			if e.Attacker.Playable() && !n.HateList().Empty() {
				// With no top desire target the cast check fails and the
				// attack check refuses: only the spells are given up.
				drainSpell(n, n.TopDesireTarget())
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			base.ClanAttacked(s, e)
			called, attacker := e.Called, e.Attacker
			if called.LifeTime() > 7 && attacker.Playable() {
				f0 := damageHate(called, float64(e.Damage), defaultnpc.HateRatio(called, attacker))
				if called.HateList().Empty() {
					called.AddAttackDesire(attacker, f0*30+300)
				} else {
					called.AddAttackDesire(attacker, f0*30)
				}
			}
			if !called.HateList().Empty() {
				if top := called.TopDesireTarget(); top != nil {
					drainSpell(called, top)
				} else {
					drainSpell(called, attacker)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			n, c := e.NPC, e.Creature
			switch {
			case c.Dead() && c.Summon():
				drainSummonKilled(n, c)
			case n.Distance2D(c) < float64(n.IntParamOr("SkillRange", drainSkillRange)):
				if !n.HateList().Empty() && n.Int(script.SlotIntAI0) != 1 {
					drainSpell(n, c)
				}
			default:
				n.RemoveAllDesire()
			}
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			n := e.NPC
			p, ok := e.Creature.(*script.Player)
			if !ok {
				return
			}
			switch {
			case p.HasItem(drainPass1) || p.HasItem(drainPass2) || p.HasItem(drainPass3):
				castByType(n, p, npc.SkillTypeMagicHeal, 1000000)
				n.RemoveAttackDesire(p)
				n.HateList().StopHate(p)
			case n.LifeTime() > 7 && n.InMyTerritory():
				drainSpell(n, p)
				n.HateList().Add(p, 100)
			}
		},
		OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
			n := e.NPC
			if home := n.SpawnLocation(); home.X != int(e.X) || home.Y != int(e.Y) || home.Z != int(e.Z) {
				n.AddMoveToDesire(n.SpawnLocation(), 30)
			} else {
				n.AddDoNothingDesire(40, 30)
			}
		},
		OnAttackFinished: func(_ *script.Script, e script.AttackFinished) {
			if c := e.Target; c.Dead() && c.Summon() {
				drainSummonKilled(e.NPC, c)
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			if k := s.Skill(e.Skill); k.AggroPoints() > 0 || k.Power(n) > 0 || k.Offensive() {
				n.AddAttackDesire(caster, defaultnpc.HateRatio(n, caster)*150)
			}
		},
	})}
}

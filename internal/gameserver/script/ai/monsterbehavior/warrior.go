package monsterbehavior

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// The weights a MonsterBehavior child's attack desires and hate take, per
// unit of damage or per event.
const (
	attackBoostValue         = 300.0
	attackedWeightPoint      = 10.0
	clanAttackedWeightPoint  = 1.0
	partyAttackedWeightPoint = 1.0
	seeSpellWeightPoint      = 10.0
)

// WarriorBehavior is the WarriorBehavior behavior. It binds no NPC of its
// own.
func WarriorBehavior() script.Script {
	return script.Script{Dir: "ai/WarriorBehavior", Behavior: true, Hooks: WarriorHooks()}
}

// attack makes n want to attack target at weight; top is n's top desire
// target as the event began. A monster that moves to attack (MovingAttack
// 1, the default) chases target. With a top desire target, a hurt one that
// failed to find its path more than 10 times jumps onto target when target
// is that top target, and a rooted one more than 40 from its top target
// drops that target unless it may still attack it, then wants target
// again. A monster that holds its ground wants target only when it may
// attack it on its own, and else drops target when target is its top
// desire target.
func attack(n *script.NPC, target script.Creature, weight float64, top script.Creature) {
	if n.IntParamOr("MovingAttack", 1) != 1 {
		if n.CanAutoAttack(target) {
			n.AddAttackDesireHold(target, weight)
		} else if top != nil && script.Same(target, top) {
			n.RemoveAttackDesire(top)
		}
		return
	}
	n.AddAttackDesire(target, weight)
	if top == nil {
		return
	}
	if n.GeoPathFailCount() > 10 && script.Same(target, top) && n.HPRatio() < 1 {
		n.TeleportTo(target)
	}
	if n.Rooted() && n.Distance2D(top) > 40 {
		if !n.CanAutoAttack(top) {
			n.RemoveAttackDesire(top)
		}
		n.AddAttackDesire(target, weight)
	}
}

// WarriorHooks are WarriorBehavior's hooks, which its children derive
// theirs from. A warrior that is no healer (IsHealer 1) attacks:
//   - its attacker, by 10 per point of damage; a party leader (Party_Type
//     2) hit by its top desire target may first call its privates onto that
//     target, by its Party_OneShot chance in 10000;
//   - the attacker of a clan member, once its Aggressive_Time has passed,
//     and of a party member as its Party_Type and Party_Loyalty say, by 1
//     per point of damage;
//   - the owner of a summon its attack killed, at 5000;
//   - a playable it sees, at 300, when aggressive (IsAggressive) and past
//     its Aggressive_Time;
//   - while in combat, the caster of a hostile or aggro-raising spell it
//     sees, at 1000.
//
// A private that receives its master's call drops its attack desires and
// answers the named target as if hit by it for 100.
func WarriorHooks() script.Hooks {
	base := MonsterHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if n.IntParam("IsHealer") == 1 {
				base.Attacked(s, e)
				return
			}
			top := n.TopDesireTarget()
			if n.IntParamOr("MovingAttack", 1) == 1 && n.IntParam("Party_Type") == 2 && int32(s.Rnd(10000)) < n.IntParam("Party_OneShot") && top != nil {
				if script.Same(attacker, top) {
					s.BroadcastScriptEventEx(n, 15000001, n.ObjectID(), top.ObjectID(), 500)
				}
			}
			attack(n, attacker, float64(e.Damage)*attackedWeightPoint, top)
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if called.LifeTime() < called.IntParam("Aggressive_Time") {
				base.ClanAttacked(s, e)
				return
			}
			partyType, partyLoyalty := called.IntParam("Party_Type"), called.IntParam("Party_Loyalty")
			if (partyType == 0 || (partyType == 1 && partyLoyalty == 0) || partyType == 2) && called.IntParam("IsHealer") != 1 {
				attack(called, e.Attacker, float64(e.Damage)*clanAttackedWeightPoint, called.TopDesireTarget())
			}
			base.ClanAttacked(s, e)
		},
		OnAttackFinished: func(_ *script.Script, e script.AttackFinished) {
			n, target := e.NPC, e.Target
			if !target.Dead() || !target.Summon() {
				return
			}
			top := n.TopDesireTarget()
			if master := script.ActingPlayer(target); master != nil {
				attack(n, master, 500*attackedWeightPoint, top)
			}
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called := e.Caller, e.Called
			partyType, partyLoyalty := called.IntParam("Party_Type"), called.IntParam("Party_Loyalty")
			if partyType == 0 || !called.HasMaster() {
				base.PartyAttacked(s, e)
				return
			}
			master := called.Master()
			if (partyType == 1 && (partyLoyalty == 0 || partyLoyalty == 1)) || (partyType == 1 && partyLoyalty == 2 && script.Same(caller, master)) || (partyType == 2 && !script.Same(caller, master)) {
				if called.IntParam("IsHealer") != 1 {
					attack(called, e.Target, float64(e.Damage)*partyAttackedWeightPoint, called.TopDesireTarget())
				}
			}
			base.PartyAttacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if c.Playable() && n.IntParam("IsAggressive") != 0 && n.LifeTime() > n.IntParam("Aggressive_Time") {
				if n.IntParamOr("MovingAttack", 1) == 1 {
					n.AddAttackDesire(c, 300)
				} else {
					n.AddAttackDesireHold(c, 300)
				}
			}
			base.SeeCreature(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n := e.NPC
			k := s.Skill(e.Skill)
			if len(e.Targets) > 0 && (k.AggroPoints() > 0 || k.Power(n) > 0 || k.Offensive()) && n.InCombat() {
				n.AddAttackDesire(e.Caster, 100*seeSpellWeightPoint)
			}
			base.SeeSpell(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if e.EventID == 15000001 {
				if c0 := s.Object(e.Arg1); c0 != nil && script.Same(c0, n.Master()) {
					if c1 := s.Object(e.Arg2); c1 != nil {
						n.RemoveAllAttackDesire()
						s.Attacked(s, script.Attacked{NPC: n, Attacker: c1, Damage: 100})
					}
				}
			}
			base.ScriptEvent(s, e)
		},
	}).Named("WarriorBehavior")
}

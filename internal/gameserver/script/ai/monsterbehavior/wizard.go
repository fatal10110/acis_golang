package monsterbehavior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// WizardBehavior is the WizardBehavior behavior. It binds no NPC of its
// own.
func WizardBehavior() script.Script {
	return script.Script{Dir: "ai/WizardBehavior", Behavior: true, Hooks: WizardHooks()}
}

// addHate adds amount to the hate c holds in n's hate list, and the 300
// opening boost on top of it while that list is empty.
func addHate(n *script.NPC, c script.Creature, amount float64) {
	hateList := n.HateList()
	if hateList.Size() == 0 {
		hateList.Add(c, amount+attackBoostValue)
	} else {
		hateList.Add(c, amount)
	}
}

// WizardHooks are WizardBehavior's hooks, which its children derive theirs
// from. A wizard fills its hate list, not its attack desires:
//   - with a playable attacker, by 10 per point of damage;
//   - past its Aggressive_Time, with the playable attacker of a clan
//     member, and of a party member as its Party_Type and Party_Loyalty
//     say, by 1 per point of damage;
//   - with a playable it sees, by 100, when aggressive (IsAggressive) and
//     no younger than its Aggressive_Time;
//   - with the caster of a hostile or aggro-raising spell it sees, by 10.
//
// Every 10 s its hate list forgets the creatures it lost and the ones 2000
// or more away. A wizard hit while silenced waits for the silence to end,
// then drops its attack desires and answers its most hated as if hit by it
// for 100.
func WizardHooks() script.Hooks {
	base := MonsterHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if n.IntParam("AttackRange") == 2 {
				n.SetInt(script.SlotIntAI4, 0)
			}
			s.StartTimerAtFixedRate("5002", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				addHate(n, attacker, float64(e.Damage)*attackedWeightPoint)
			}
			if n.Muted() {
				n.SetInt(script.SlotIntAI4, 1)
				s.StartTimer("5001", n, nil, 10000*time.Millisecond)
			}
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "5001"):
				if n.Muted() {
					s.StartTimer("5001", n, nil, 10000*time.Millisecond)
				} else {
					n.RemoveAllAttackDesire()
					n.SetInt(script.SlotIntAI4, 0)
					if mostHated := n.HateList().MostHated(); mostHated != nil {
						s.Attacked(s, script.Attacked{NPC: n, Attacker: mostHated, Damage: 100})
					}
				}
			case strings.EqualFold(e.Name, "5002"):
				n.HateList().Refresh()
				n.HateList().RemoveIfOutOfRange(2000)
			}
			return base.Timer(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.LifeTime() > called.IntParam("Aggressive_Time") {
				partyType, partyLoyalty := called.IntParam("Party_Type"), called.IntParam("Party_Loyalty")
				if ((partyType == 0 || (partyType == 1 && partyLoyalty == 0)) || partyType == 2) && attacker.Playable() {
					addHate(called, attacker, float64(e.Damage)*clanAttackedWeightPoint)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called, target := e.Caller, e.Called, e.Target
			if called.LifeTime() > called.IntParam("Aggressive_Time") {
				partyType, partyLoyalty := called.IntParam("Party_Type"), called.IntParam("Party_Loyalty")
				master := called.Master()
				if (((partyType == 1 && (partyLoyalty == 0 || partyLoyalty == 1)) || (partyType == 1 && partyLoyalty == 2 && script.Same(caller, master))) || (partyType == 2 && !script.Same(caller, master))) && target.Playable() {
					addHate(called, target, float64(e.Damage)*partyAttackedWeightPoint)
				}
			}
			base.PartyAttacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if c.Playable() && n.IntParam("IsAggressive") != 0 && n.LifeTime() >= n.IntParam("Aggressive_Time") {
				hateList := n.HateList()
				if hateList.Size() == 0 {
					hateList.Add(c, (100*partyAttackedWeightPoint)+300)
				} else {
					hateList.Add(c, 100*partyAttackedWeightPoint)
				}
			}
			base.SeeCreature(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			k := s.Skill(e.Skill)
			if len(e.Targets) > 0 && (k.AggroPoints() > 0 || k.Power(n) > 0 || k.Offensive()) {
				hateList := n.HateList()
				if n.Intention() == script.IntentionAttack && script.Same(caster, n.TopDesireTarget()) {
					hateList.Add(caster, attackedWeightPoint)
				} else {
					hateList.Add(caster, seeSpellWeightPoint)
				}
			}
			base.SeeSpell(s, e)
		},
	}).Named("WizardBehavior")
}

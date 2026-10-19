package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// debuffMostHated makes n want to lay its debuff on c when a draw under 33
// passes, c holds no effect of its kind and c is the creature mostHated
// names.
func debuffMostHated(s *script.Script, n *script.NPC, c, mostHated script.Creature) {
	debuff, ok := n.SkillByType(npc.SkillTypeDebuff)
	if s.Rnd(100) < 33 && s.AbnormalLevel(c, debuff) <= 0 && script.Same(mostHated, c) && ok {
		n.AddCastDesire(c, debuff, 1000000)
	}
}

// PartyPrivateCast3SkillsMagical is PartyPrivateCast3SkillsMagical: a
// PartyPrivateWarrior that may cast either of its DD magics and its debuff
// on its most hated attacker standing more than 100 away, and on a clan
// member's attacker while it is not attacking. It binds no NPC of its own.
func PartyPrivateCast3SkillsMagical() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() && n.Distance2D(attacker) > 100 {
				if script.Same(n.MostHated(), attacker) {
					if s.Rnd(100) < 33 {
						castByType(n, attacker, npc.SkillTypeDDMagic1)
					}
					if s.Rnd(100) < 33 {
						castByType(n, attacker, npc.SkillTypeDDMagic2)
					}
					maybeDebuff(s, n, attacker, npc.SkillTypeDebuff)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack {
				if called.Distance2D(attacker) > 100 {
					if s.Rnd(100) < 33 {
						castByType(called, attacker, npc.SkillTypeDDMagic1)
					}
					if s.Rnd(100) < 33 {
						castByType(called, attacker, npc.SkillTypeDDMagic2)
					}
				}
				maybeDebuff(s, called, attacker, npc.SkillTypeDebuff)
			}
			base.ClanAttacked(s, e)
		},
	})}
}

// PartyPrivateCastCurse is PartyPrivateCastCurse: a PartyPrivateWarrior
// that may lay its debuff on its most hated attacker and on a clan
// member's attacker.
func PartyPrivateCastCurse() script.Script {
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{20982}, Hooks: PartyPrivateCastCurseHooks()}
}

// PartyPrivateCastCurseHooks are PartyPrivateCastCurse's hooks, which its
// children derive theirs from.
func PartyPrivateCastCurseHooks() script.Hooks {
	base := PartyPrivateHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if mostHated := n.MostHated(); mostHated != nil {
					debuffMostHated(s, n, attacker, mostHated)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if e.Called.LifeTime() > 7 && e.Attacker.Playable() {
				maybeDebuff(s, e.Called, e.Attacker, npc.SkillTypeDebuff)
			}
			base.ClanAttacked(s, e)
		},
	}).Named("PartyPrivateCastCurse")
}

// PartyPrivateHereticCastCurse is PartyPrivateHereticCastCurse: a
// PartyPrivateCastCurse that keeps to its territory until it is drawn into
// a fight. Hit, or told of a hit on its party, while neither attacking nor
// casting, it gives up the territory; out of the territory before that it
// drops its attack desires and walks back to its spawn. Its master's
// events 10002 and 10033 turn it on the creature they name (10033 also
// releases it from the territory), and 10035 sends it home bound to the
// territory again.
func PartyPrivateHereticCastCurse() script.Script {
	base := PartyPrivateCastCurseHooks()
	idle := func(n *script.NPC) bool {
		i := n.Intention()
		return i != script.IntentionAttack && i != script.IntentionCast
	}
	goHome := func(n *script.NPC) {
		n.RemoveAllAttackDesire()
		n.AddMoveToDesire(n.SpawnLocation(), 100)
	}
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{22157}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if n := e.NPC; idle(n) && n.Int(script.SlotIntAI4) == 0 {
				n.SetInt(script.SlotIntAI4, 1)
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if called := e.Called; idle(called) && called.Int(script.SlotIntAI4) == 0 {
				called.SetInt(script.SlotIntAI4, 1)
			}
			base.PartyAttacked(s, e)
		},
		OnOutOfTerritory: func(_ *script.Script, e script.OutOfTerritory) {
			if e.NPC.Int(script.SlotIntAI4) == 0 {
				goHome(e.NPC)
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			master := n.Master()
			if master == nil || e.Arg2 != master.ObjectID() {
				return
			}
			switch e.EventID {
			case 10033, 10002:
				c0 := s.Object(e.Arg1)
				if c0 == nil {
					return
				}
				if e.EventID == 10033 {
					n.SetInt(script.SlotIntAI4, 1)
				}
				n.RemoveAllAttackDesire()
				if c0.Playable() {
					n.AddAttackDesire(c0, 200)
				}
			case 10035:
				n.SetInt(script.SlotIntAI4, 0)
				goHome(n)
			}
		},
	})}
}

// PartyPrivateCastDDHeal is PartyPrivateCastDDHeal: a PartyPrivateWarrior
// that may heal itself or a party member under 70% of its HP, and cast its
// DD magic on an attacker standing more than 100 away, its own only when
// it is the creature it last took up.
func PartyPrivateCastDDHeal() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{20948, 20943, 20987, 20975, 20764, 20981, 21059, 21074, 21080}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if s.Rnd(100) < 33 && n.HPRatio() < 0.7 {
				castByType(n, n, npc.SkillTypeMagicHeal)
			}
			if attacker.Playable() && n.Distance2D(attacker) > 100 {
				if script.Same(n.TopDesireTarget(), attacker) && s.Rnd(100) < 33 {
					castByType(n, attacker, npc.SkillTypeDDMagic)
				}
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called, target := e.Caller, e.Called, e.Target
			if !script.Same(caller, called) && s.Rnd(100) < 33 && caller.HPRatio() < 0.7 {
				castByType(called, caller, npc.SkillTypeMagicHeal)
			}
			if target.Playable() && called.Distance2D(target) > 100 && s.Rnd(100) < 33 {
				castByType(called, target, npc.SkillTypeDDMagic)
			}
			base.PartyAttacked(s, e)
		},
	})}
}

// PartyPrivateCastDDMagicCurseStone is PartyPrivateCastDDMagicCurseStone:
// a Ketra or Varka PartyPrivateWarrior that petrifies an attacker allied
// with its own faction, and a player who helps one. Otherwise it may lay
// its debuff on its most hated attacker, its own or its party's, and cast
// its DD magic on its attacker, and on its party's while it is not
// attacking.
func PartyPrivateCastDDMagicCurseStone() script.Script {
	base := PartyPrivateHooks()
	return script.Script{Dir: partyPrivateDir, Behavior: true, NPCs: []int32{21349, 21375}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if defaultnpc.MaybeCastPetrify(n, attacker) {
					return
				}
				if mostHated := n.MostHated(); mostHated != nil {
					debuffMostHated(s, n, attacker, mostHated)
				}
			}
			if s.Rnd(100) < 33 {
				castByType(n, attacker, npc.SkillTypeDDMagic)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if defaultnpc.MaybeCastPetrify(e.Called, e.Attacker) {
				return
			}
			base.ClanAttacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			if s.Rnd(100) < 33 && called.Intention() != script.IntentionAttack {
				castByType(called, target, npc.SkillTypeDDMagic)
			}
			if target.Playable() {
				if mostHated := called.MostHated(); mostHated != nil {
					debuffMostHated(s, called, target, mostHated)
				}
			}
			base.PartyAttacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if k := s.Skill(e.Skill); k.AggroPoints() > 0 && !k.Offensive() && defaultnpc.MaybeCastPetrify(e.NPC, e.Caster) {
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}

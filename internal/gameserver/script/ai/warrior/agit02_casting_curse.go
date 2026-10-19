package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// flagHate is the hate a clan hall siege NPC adds for amount, a hit's
// damage or a skill's aggro points: ratio for each twentieth of its
// maximum hit points.
func flagHate(n *script.NPC, amount int32, ratio float64) float64 {
	return float64(amount) / float64(n.MaxHP()) / 0.05 * ratio
}

// Agit02DoomGuardAgit is Agit02DoomGuardAgit: a clan hall siege guard, a
// WarriorCastingCurse that never wanders and holds its ground. It wants to
// attack every player and summon it sees, each one that hits it by the
// damage, and each caster of a skill with aggro by the aggro, and may lay
// its debuff on the attacker it last took up. It answers only the clan
// members that share its flag.
func Agit02DoomGuardAgit() script.Script {
	base := CastingCurseHooks()
	return script.Script{Dir: castingCurseDir, Behavior: true, NPCs: []int32{35412, 35633}, Hooks: base.With(script.Hooks{
		// With no desire left, it does nothing.
		OnNoDesire: func(*script.Script, script.NoDesire) {},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotFlag, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				top := n.TopDesireTarget()
				debuff, ok := n.SkillByType(npc.SkillTypeDebuff)
				if top != nil && s.Rnd(100) < 10 && s.AbnormalLevel(attacker, debuff) <= 0 && script.Same(top, attacker) && ok {
					n.AddCastDesireHold(top, debuff, 1000000)
				}
			}
			if attacker.Playable() {
				n.AddAttackDesireHold(attacker, flagHate(n, e.Damage, 100))
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called, attacker := e.Caller, e.Called, e.Attacker
			sameFlag := called.Int(script.SlotFlag) == caller.Int(script.SlotFlag)
			if called.LifeTime() > 7 && attacker.Playable() && sameFlag {
				debuff, ok := called.SkillByType(npc.SkillTypeDebuff)
				if s.Rnd(100) < 10 && s.AbnormalLevel(attacker, debuff) <= 0 && ok {
					called.AddCastDesireHold(attacker, debuff, 1000000)
				}
			}
			if sameFlag && attacker.Playable() {
				called.AddAttackDesireHold(attacker, flagHate(called, e.Damage, 100))
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if aggro := s.Skill(e.Skill).AggroPoints(); aggro > 0 {
				e.NPC.AddAttackDesireHold(e.Caster, flagHate(e.NPC, int32(aggro), 150))
			}
		},
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			e.NPC.AddAttackDesireHold(e.Creature, 200)
		},
	})}
}

// Agit02DoomKnightAgitParty is Agit02DoomKnightAgitParty: a clan hall
// siege knight, a WarriorCastingCurse private that never wanders and
// ignores its clan. Every 60 s it may jump to its master when out of its
// territory and may shuffle its hate. It answers its party: it may lay its
// debuff on a party member's attacker, and wants to attack it by the
// damage when the member shares its flag. A skill with aggro makes it want
// to attack the caster by the aggro. It binds no NPC of its own.
func Agit02DoomKnightAgitParty() script.Script {
	return script.Script{Dir: castingCurseDir, Behavior: true, Hooks: Agit02DoomKnightAgitPartyHooks()}
}

// Agit02DoomKnightAgitPartyHooks are Agit02DoomKnightAgitParty's hooks,
// which its children derive theirs from.
func Agit02DoomKnightAgitPartyHooks() script.Hooks {
	base := CastingCurseHooks()
	return base.With(script.Hooks{
		// With no desire left, it does nothing.
		OnNoDesire: func(*script.Script, script.NoDesire) {},
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("3001", e.NPC, nil, 1000*time.Millisecond, 60000*time.Millisecond)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3001") {
				n := e.NPC
				if !n.InMyTerritory() && s.Rnd(3) < 1 {
					if master := n.Master(); master != nil {
						n.TeleportTo(master)
						n.RemoveAllAttackDesire()
					}
				}
				if s.Rnd(5) < 1 {
					n.RandomizeAttack()
				}
			}
			return base.Timer(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called, target := e.Caller, e.Called, e.Target
			if called.LifeTime() > 8 && target.Playable() {
				// The debuff's effect is looked up on the attacked party
				// member, not on its attacker.
				debuff, ok := called.SkillByType(npc.SkillTypeDebuff)
				if s.Rnd(100) < 10 && s.AbnormalLevel(caller, debuff) <= 0 && ok {
					called.AddCastDesire(target, debuff, 1000000)
				}
			}
			if called.Int(script.SlotFlag) == caller.Int(script.SlotFlag) && target.Playable() {
				called.AddAttackDesire(target, flagHate(called, e.Damage, 100))
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if aggro := s.Skill(e.Skill).AggroPoints(); aggro > 0 {
				e.NPC.AddAttackDesire(e.Caster, flagHate(e.NPC, int32(aggro), 150))
			}
		},
		// It ignores its clan.
		OnClanAttacked: func(*script.Script, script.ClanAttacked) {},
	}).Named("Agit02DoomKnightAgitParty")
}

// Agit02VampireAgitParty is Agit02VampireAgitParty: an
// Agit02DoomKnightAgitParty that ignores its party and answers its clan
// instead, the way the knight answers its party.
func Agit02VampireAgitParty() script.Script {
	base := Agit02DoomKnightAgitPartyHooks()
	return script.Script{Dir: castingCurseDir, Behavior: true, NPCs: []int32{35647}, Hooks: base.With(script.Hooks{
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called, attacker := e.Caller, e.Called, e.Attacker
			if called.LifeTime() > 8 && attacker.Playable() {
				// The debuff's effect is looked up on the attacked clan
				// member, not on its attacker.
				debuff, ok := called.SkillByType(npc.SkillTypeDebuff)
				if s.Rnd(100) < 10 && s.AbnormalLevel(caller, debuff) <= 0 && ok {
					called.AddCastDesire(attacker, debuff, 1000000)
				}
			}
			if called.Int(script.SlotFlag) == caller.Int(script.SlotFlag) && attacker.Playable() {
				called.AddAttackDesire(attacker, flagHate(called, e.Damage, 100))
			}
		},
		// It ignores its party.
		OnPartyAttacked: func(*script.Script, script.PartyAttacked) {},
	})}
}

// Agit02DoomKnightAgit is Agit02DoomKnightAgit: a clan hall siege knight,
// a WarriorCastingCurseAggressive that never wanders. Every 60 s it may
// jump home when out of its territory and may shuffle its hate. It answers
// only the clan members that share its flag: it may lay its debuff, from
// where it stands, on their attacker, and wants to attack it by the damage.
func Agit02DoomKnightAgit() script.Script {
	base := CastingCurseAggressiveHooks()
	return script.Script{Dir: castingCurseAggressiveDir, Behavior: true, NPCs: []int32{35416, 35637}, Hooks: base.With(script.Hooks{
		// With no desire left, it does nothing.
		OnNoDesire: func(*script.Script, script.NoDesire) {},
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("3001", e.NPC, nil, 1000*time.Millisecond, 60000*time.Millisecond)
			e.NPC.SetInt(script.SlotFlag, 0)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3001") {
				n := e.NPC
				if !n.InMyTerritory() && s.Rnd(3) < 1 {
					n.TeleportTo(n.SpawnLocation())
					n.RemoveAllAttackDesire()
				}
				if s.Rnd(5) < 1 {
					n.RandomizeAttack()
				}
			}
			return base.Timer(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called, attacker := e.Caller, e.Called, e.Attacker
			sameFlag := called.Int(script.SlotFlag) == caller.Int(script.SlotFlag)
			if called.LifeTime() > 7 && attacker.Playable() && sameFlag {
				debuff, ok := called.SkillByType(npc.SkillTypeDebuff)
				if s.Rnd(100) < 10 && s.AbnormalLevel(attacker, debuff) <= 0 && ok {
					called.AddCastDesireHold(attacker, debuff, 1000000)
				}
			}
			if sameFlag && attacker.Playable() {
				called.AddAttackDesire(attacker, flagHate(called, e.Damage, 100))
			}
		},
	})}
}

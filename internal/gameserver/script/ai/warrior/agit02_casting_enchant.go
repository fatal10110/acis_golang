package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Agit02DoomArcherAgit is Agit02DoomArcherAgit: a clan hall siege archer,
// a WarriorCastingEnchantClan that never wanders and holds its ground. It
// may once buff itself when a player or summon first hits it, and buff a
// clan member that shares its flag; it wants to attack every player and
// summon it sees, each one that hits it or a member of its flag by the
// damage, each caster of a skill with aggro by the aggro, and each
// creature that hits its door: a siege summon's owner above the summon
// itself.
func Agit02DoomArcherAgit() script.Script {
	base := CastingEnchantClanHooks()
	return script.Script{Dir: castingEnchantDir, Behavior: true, NPCs: []int32{35413}, Hooks: base.With(script.Hooks{
		// With no desire left, it does nothing.
		OnNoDesire: func(*script.Script, script.NoDesire) {},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotFlag, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				buff, ok := n.SkillByType(npc.SkillTypeBuff)
				if n.Int(script.SlotIntAI1) == 0 && s.Rnd(100) < 33 && n.HPRatio() > 0.5 && ok {
					n.AddCastDesire(n, buff, 1000000)
				}
				n.SetInt(script.SlotIntAI1, 1)
			}
			if attacker.Playable() {
				n.AddAttackDesireHold(attacker, flagHate(n, e.Damage, 100))
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called := e.Caller, e.Called
			buffFlagMember(s, caller, called)
			if called.Int(script.SlotFlag) == caller.Int(script.SlotFlag) && e.Attacker.Playable() {
				called.AddAttackDesireHold(e.Attacker, flagHate(called, e.Damage, 100))
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if aggro := s.Skill(e.Skill).AggroPoints(); aggro > 0 {
				e.NPC.AddAttackDesireHold(e.Caster, flagHate(e.NPC, int32(aggro), 150))
			}
		},
		OnStaticObjectClanAttacked: func(_ *script.Script, e script.StaticObjectClanAttacked) {
			switch {
			case e.Attacker.SiegeSummon():
				e.Called.AddAttackDesire(script.ActingPlayer(e.Attacker), 5000)
				e.Called.AddAttackDesire(e.Attacker, 1000)
			case e.Attacker.Playable():
				e.Called.AddAttackDesire(e.Attacker, flagHate(e.Called, e.Damage, 50))
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

// buffFlagMember is the buff half of a clan hall siege archer's answer to
// a member's call: not attacking and with its buff unspent, it may buff
// the member caller of its flag, still above half its hit points, unless
// the buff already holds on it. Any call spends the buff.
func buffFlagMember(s *script.Script, caller, called *script.NPC) {
	if called.Intention() != script.IntentionAttack && called.Int(script.SlotIntAI1) == 0 &&
		called.Int(script.SlotFlag) == caller.Int(script.SlotFlag) {
		buff, ok := called.SkillByType(npc.SkillTypeBuff)
		if s.Rnd(100) < 50 && s.AbnormalLevel(caller, buff) <= 0 && caller.HPRatio() > 0.5 && ok {
			called.AddCastDesire(caller, buff, 1000000)
		}
	}
	called.SetInt(script.SlotIntAI1, 1)
}

// Agit02DoomArcherAgitParty is Agit02DoomArcherAgitParty: a clan hall
// siege archer, a WarriorCastingEnchantClan private that never wanders and
// ignores its clan. Every 60 s it may jump to its master when out of its
// territory and may shuffle its hate. It may once buff itself when a
// player or summon first hits it, which adds no hate. It answers its
// party: it may buff a party member of its flag, and wants to attack its
// attacker by the damage, held in place. A skill with aggro makes it want
// to attack the caster by the aggro.
func Agit02DoomArcherAgitParty() script.Script {
	base := CastingEnchantClanHooks()
	return script.Script{Dir: castingEnchantDir, Behavior: true, NPCs: []int32{35646}, Hooks: base.With(script.Hooks{
		// With no desire left, it does nothing.
		OnNoDesire: func(*script.Script, script.NoDesire) {},
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("4001", e.NPC, nil, 1000*time.Millisecond, 60000*time.Millisecond)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "4001") {
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
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if e.Attacker.Playable() {
				buff, ok := n.SkillByType(npc.SkillTypeBuff)
				if n.Int(script.SlotIntAI1) == 0 && s.Rnd(100) < 33 && n.HPRatio() > 0.5 && ok {
					n.AddCastDesire(n, buff, 1000000)
				}
				n.SetInt(script.SlotIntAI1, 1)
			}
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called := e.Caller, e.Called
			buffFlagMember(s, caller, called)
			if called.Int(script.SlotFlag) == caller.Int(script.SlotFlag) && e.Target.Playable() {
				called.AddAttackDesireHold(e.Target, flagHate(called, e.Damage, 100))
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if aggro := s.Skill(e.Skill).AggroPoints(); aggro > 0 {
				e.NPC.AddAttackDesireHold(e.Caster, flagHate(e.NPC, int32(aggro), 150))
			}
		},
		// It ignores its clan.
		OnClanAttacked: func(*script.Script, script.ClanAttacked) {},
	})}
}

// Agit02DoomWarriorAgit is Agit02DoomWarriorAgit: a clan hall siege
// warrior, a WarriorCastingEnchantClanAggressive that never wanders. Every
// 60 s it may jump home when out of its territory and may shuffle its
// hate. It answers only the clan members that share its flag, wanting to
// attack their attacker by the damage.
func Agit02DoomWarriorAgit() script.Script {
	base := CastingEnchantClanAggressiveHooks()
	return script.Script{Dir: castingEnchantDir, Behavior: true, NPCs: []int32{35415, 35636}, Hooks: base.With(script.Hooks{
		// With no desire left, it does nothing.
		OnNoDesire: func(*script.Script, script.NoDesire) {},
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("4001", e.NPC, nil, 1000*time.Millisecond, 60000*time.Millisecond)
			e.NPC.SetInt(script.SlotFlag, 0)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "4001") {
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
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			if e.Called.Int(script.SlotFlag) == e.Caller.Int(script.SlotFlag) && e.Attacker.Playable() {
				e.Called.AddAttackDesire(e.Attacker, flagHate(e.Called, e.Damage, 100))
			}
		},
	})}
}

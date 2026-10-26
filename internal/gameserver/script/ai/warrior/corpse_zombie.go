package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const corpseZombieDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCorpseZombie"

// jumpOnto aborts what n does, puts it on target in one step and makes it
// want its jump skill (4671-1) on target.
func jumpOnto(n *script.NPC, target script.Creature) {
	n.AbortAll()
	n.InstantTeleportTo(target)
	n.AddCastDesire(target, skill.Ref{ID: 4671, Level: 1}, 1000000)
}

// CorpseZombie is WarriorCorpseZombie: a Warrior that, as a private
// (IsPrivate 1), follows its living master when it has nothing to do,
// wanders otherwise, and leaves the world while it fights no one once its
// master died. Set to jump (IsTeleport), on a draw under 10 it jumps onto
// the attacker farther than 100 that it hates most, or onto its clan's
// attacker farther than 100 while it does not fight. Once below 10% of its
// HP, and on a draw under 10 when it dies, it casts its self-centred
// debuff.
func CorpseZombie() script.Script {
	return script.Script{Dir: corpseZombieDir, Behavior: true, NPCs: []int32{21554, 21548}, Hooks: corpseZombieHooks()}
}

// corpseZombieHooks are WarriorCorpseZombie's hooks, which its children
// derive theirs from.
func corpseZombieHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, 0)
			if n.IntParam("IsPrivate") == 1 {
				s.StartTimerAtFixedRate("1004", n, nil, 20000*time.Millisecond, 20000*time.Millisecond)
			}
			base.Created(s, e)
		},
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			n := e.NPC
			if n.IntParam("IsPrivate") == 1 {
				if master := n.Master(); master != nil && !master.Dead() {
					n.AddFollowDesire(master, 50)
				} else {
					n.AddWanderDesire(5, 5)
				}
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1004") {
				n := e.NPC
				if deadMaster(n) && n.IntParam("IsPrivate") == 1 && !fighting(n.Intention()) {
					s.Spawner().DecayMe(n)
					return ""
				}
			}
			return base.Timer(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			if s.Rnd(100) < 10 {
				castByType(n, n, npc.SkillTypeSelfRangeDebuff)
			}
			base.MyDying(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if n.IntParam("IsTeleport") != 0 && n.Distance2D(attacker) > 100 {
				if script.Same(n.MostHated(), attacker) && s.Rnd(100) < 10 {
					jumpOnto(n, attacker)
					if attacker.Playable() {
						f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
						n.AddAttackDesire(attacker, f0*30)
					}
				}
			}
			if n.HPRatio() < 0.1 && n.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 10 {
				castByType(n, n, npc.SkillTypeSelfRangeDebuff)
				n.SetInt(script.SlotIntAI0, 1)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if called.IntParam("IsTeleport") != 0 {
				if called.Intention() != script.IntentionAttack && called.Distance2D(attacker) > 100 && s.Rnd(100) < 10 {
					jumpOnto(called, attacker)
					if attacker.Playable() {
						f0 := damageHate(called, float64(e.Damage), defaultnpc.HateRatio(called, attacker))
						called.AddAttackDesire(attacker, float64(int(f0*30)))
					}
				}
			}
			base.ClanAttacked(s, e)
		},
	}).Named("WarriorCorpseZombie")
}

// CorpseZombieDDMagic is WarriorCorpseZombieDDMagic: a WarriorCorpseZombie
// that, on a draw under 33, wants its damage magic on a player or summon
// hitting it from farther than 100 that it most wants to act on, or on its
// clan's attacker farther than 100 while it does not fight.
func CorpseZombieDDMagic() script.Script {
	return script.Script{Dir: corpseZombieDir, Behavior: true, NPCs: []int32{21553, 21597, 21600}, Hooks: corpseZombieDDMagicHooks()}
}

// corpseZombieDDMagicHooks are WarriorCorpseZombieDDMagic's hooks, which
// its children derive theirs from.
func corpseZombieDDMagicHooks() script.Hooks {
	base := corpseZombieHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() && n.Distance2D(attacker) > 100 {
				if top := n.TopDesireTarget(); top != nil && script.Same(top, attacker) {
					if s.Rnd(100) < 33 {
						castByType(n, attacker, npc.SkillTypeDDMagic)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack {
				if called.Distance2D(attacker) > 100 && s.Rnd(100) < 33 {
					castByType(called, attacker, npc.SkillTypeDDMagic)
				}
			}
			base.ClanAttacked(s, e)
		},
	}).Named("WarriorCorpseZombieDDMagic")
}

// CorpseZombieDDMagicAggressive is WarriorCorpseZombieDDMagicAggressive: a
// WarriorCorpseZombieDDMagic that attacks the players and summons it sees.
// Fighting no one, set to jump (IsTeleport), on a draw under 10 it jumps
// onto one it sees farther than 100, and settled in its territory, on a
// draw under 33, wants its damage magic on one it sees farther than 100.
func CorpseZombieDDMagicAggressive() script.Script {
	base := corpseZombieDDMagicHooks()
	hooks := base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				return
			}
			if n.LifeTime() > 7 && n.Intention() != script.IntentionAttack {
				if n.IntParam("IsTeleport") != 0 && n.Distance2D(c) > 100 && s.Rnd(100) < 10 {
					jumpOnto(n, c)
				}
				if n.InMyTerritory() {
					if n.Distance2D(c) > 100 && s.Rnd(100) < 33 {
						castByType(n, c, npc.SkillTypeDDMagic)
					}
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})
	return script.Script{Dir: corpseZombieDir, Behavior: true, NPCs: []int32{21409, 21570}, Hooks: hooks}
}

// CorpseZombiePhysicalSpecialAggressive is
// WarriorCorpseZombiePhysicalSpecialAggressive: a
// WarriorCorpseZombieDDMagic that attacks the players and summons it sees
// and, set to jump (IsTeleport) and fighting no one, on a draw under 10
// jumps onto one it sees farther than 100.
func CorpseZombiePhysicalSpecialAggressive() script.Script {
	base := corpseZombieDDMagicHooks()
	hooks := base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			if n.IntParam("IsTeleport") != 0 && n.LifeTime() > 7 && n.Distance2D(c) > 100 && n.Intention() != script.IntentionAttack && s.Rnd(100) < 10 {
				jumpOnto(n, c)
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})
	return script.Script{Dir: corpseZombieDir, Behavior: true, NPCs: []int32{21550, 21408, 21552, 21561, 22176}, Hooks: hooks}
}

// CorpseZombiePhysicalSpecial is WarriorCorpseZombiePhysicalSpecial: a
// WarriorCorpseZombie that, on a draw under 33, strikes with its physical
// special skill the player it hates most when hit by that one, or its
// clan's attacker while it does not fight.
func CorpseZombiePhysicalSpecial() script.Script {
	return script.Script{Dir: corpseZombieDir, Behavior: true, NPCs: []int32{21578, 21549, 21547, 21551, 21598, 21601}, Hooks: corpseZombiePhysicalSpecialHooks()}
}

// corpseZombiePhysicalSpecialHooks are WarriorCorpseZombiePhysicalSpecial's
// hooks, which its children derive theirs from.
func corpseZombiePhysicalSpecialHooks() script.Hooks {
	base := corpseZombieHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if script.Same(n.MostHated(), attacker) && s.Rnd(100) < 33 {
					castByType(n, attacker, npc.SkillTypePhysicalSpecial)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack && s.Rnd(100) < 33 {
				castByType(called, attacker, npc.SkillTypePhysicalSpecial)
			}
			base.ClanAttacked(s, e)
		},
	}).Named("WarriorCorpseZombiePhysicalSpecial")
}

// HereticCorpseZombiePhysicalSpecial is
// WarriorHereticCorpseZombiePhysicalSpecial: a
// WarriorCorpseZombiePhysicalSpecial that keeps to its territory until it
// is drawn into a fight. Hit, or told of a hit on its clan, while neither
// attacking nor casting, it gives up the territory (i_ai4); out of the
// territory before that it drops its attack desires and walks back to its
// spawn. Script events 10002 and 10033 turn it on the creature they name
// (10033 also releases it from the territory), and 10035 sends it home
// bound to the territory again.
func HereticCorpseZombiePhysicalSpecial() script.Script {
	base := corpseZombiePhysicalSpecialHooks()
	hooks := base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI4, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if hereticIdle(e.NPC) && e.NPC.Int(script.SlotIntAI4) == 0 {
				e.NPC.SetInt(script.SlotIntAI4, 1)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			if hereticIdle(e.Called) && e.Called.Int(script.SlotIntAI4) == 0 {
				e.Called.SetInt(script.SlotIntAI4, 1)
			}
			base.ClanAttacked(s, e)
		},
		OnOutOfTerritory: hereticOutOfTerritory,
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
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
				hereticGoHome(n)
			}
		},
	})
	return script.Script{Dir: corpseZombieDir, Behavior: true, NPCs: []int32{22140, 22149}, Hooks: hooks}
}

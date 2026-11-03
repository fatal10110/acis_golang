package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const slowTypeDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorSlowType"

// SlowType is WarriorSlowType: a Warrior that, once hit, checks every 5
// seconds while it attacks or casts whether the creature it hates most hit
// it since; if not, a draw under 50 wants its slowing magic on that one.
// Once it neither attacks nor casts, the checks stop. Told after its first
// 7 AI cycles that a player or a summon hit a clan member while it does
// not attack, it wants its slowing magic on that one on a draw under 10.
//
// Its memory: IntAI0 is set while its checks run; IntAI1 is set when the
// creature it hates most hit it since the last check.
func SlowType() script.Script {
	return script.Script{Dir: slowTypeDir, Behavior: true, Hooks: slowTypeHooks()}
}

// slowTypeHooks are WarriorSlowType's hooks, which its children derive
// theirs from.
func slowTypeHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			e.NPC.SetInt(script.SlotIntAI1, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.Int(script.SlotIntAI0) == 1 {
				if mostHated := n.MostHated(); mostHated != nil && script.Same(mostHated, e.Attacker) {
					n.SetInt(script.SlotIntAI1, 1)
				}
			} else {
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI0, 1)
			}
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "2001") {
				if i := n.Intention(); i != script.IntentionAttack && i != script.IntentionCast {
					n.SetInt(script.SlotIntAI0, 0)
					n.SetInt(script.SlotIntAI1, 0)
					return base.Timer(s, e)
				}
				if n.Int(script.SlotIntAI1) == 0 {
					if mostHated := n.MostHated(); mostHated != nil && s.Rnd(100) < 50 {
						castByType(n, mostHated, npc.SkillTypeDDMagicSlow)
					}
				}
				s.StartTimer("2001", n, nil, 5000*time.Millisecond)
				n.SetInt(script.SlotIntAI1, 0)
			}
			return base.Timer(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if e.Attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack && s.Rnd(100) < 10 {
				castByType(called, e.Attacker, npc.SkillTypeDDMagicSlow)
			}
			base.ClanAttacked(s, e)
		},
	}).Named("WarriorSlowType")
}

// SlowType1 is WarriorSlowType1: a WarriorSlowType that, hit while it
// attacks from within 100 by another than the creature it hates most, which
// stands farther than 100, on a draw under 80 drops its attack desires and,
// hit by a player or a summon, turns on that one. Hit from within 200, a
// draw under 10 wants its slowing magic on the creature it hates most.
func SlowType1() script.Script {
	return script.Script{Dir: slowTypeDir, Behavior: true, NPCs: []int32{20101, 20378, 20379, 20380, 20526, 21128, 21131, 20346, 20566, 20565, 20083, 20368, 20333, 21103, 20511, 20559, 20016, 20591, 20597, 20521}, Hooks: slowType1Hooks()}
}

// slowType1Hooks are WarriorSlowType1's hooks, which its children derive
// theirs from.
func slowType1Hooks() script.Hooks {
	base := slowTypeHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			dist := n.Distance2D(attacker)
			mostHated := n.MostHated()
			if mostHated != nil && !script.Same(attacker, mostHated) && n.Intention() == script.IntentionAttack && n.Distance2D(mostHated) > 100 && dist < 100 && s.Rnd(100) < 80 {
				n.RemoveAllAttackDesire()
				if attacker.Playable() {
					perLevel := float64(e.Damage) / float64(n.Level()+7)
					f0 := perLevel + defaultnpc.HateRatio(n, attacker)/100*perLevel
					n.AddAttackDesire(attacker, f0*30)
				}
			}
			// A creature hated by none takes no cast desire.
			if dist < 200 && s.Rnd(100) < 10 && mostHated != nil {
				castByType(n, mostHated, npc.SkillTypeDDMagicSlow)
			}
			base.Attacked(s, e)
		},
	}).Named("WarriorSlowType1")
}

// SlowType1Aggressive is WarriorSlowType1Aggressive: a WarriorSlowType1
// that attacks the players and summons it sees and, after its first 7 AI
// cycles while it does not attack and stands in its territory, wants its
// slowing magic on one farther than 100 on a draw under 10.
func SlowType1Aggressive() script.Script {
	return script.Script{Dir: slowTypeDir, Behavior: true, NPCs: []int32{21385, 21386, 20085, 20149, 21137}, Hooks: slowType1AggressiveHooks()}
}

// slowType1AggressiveHooks are WarriorSlowType1Aggressive's hooks.
func slowType1AggressiveHooks() script.Hooks {
	base := slowType1Hooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			if n.LifeTime() > 7 && n.Intention() != script.IntentionAttack && n.InMyTerritory() {
				if n.Distance2D(c) > 100 && s.Rnd(100) < 10 {
					castByType(n, c, npc.SkillTypeDDMagicSlow)
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})
}

// SlowType2 is WarriorSlowType2: a WarriorSlowType that, hit from farther
// than 200, wants its slowing magic on the attacker on a draw under 70.
func SlowType2() script.Script {
	return script.Script{Dir: slowTypeDir, Behavior: true, NPCs: []int32{20201, 20659, 21800, 20015, 20020, 27099, 27017, 20026, 20029, 20562, 20046, 20041, 27024, 20341, 20616, 20457, 22045}, Hooks: slowType2Hooks()}
}

// slowType2Hooks are WarriorSlowType2's hooks, which its children derive
// theirs from.
func slowType2Hooks() script.Hooks {
	base := slowTypeHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if e.NPC.Distance2D(e.Attacker) > 200 && s.Rnd(100) < 70 {
				castByType(e.NPC, e.Attacker, npc.SkillTypeDDMagicSlow)
			}
			base.Attacked(s, e)
		},
	}).Named("WarriorSlowType2")
}

// SlowType2Aggressive is WarriorSlowType2Aggressive: a WarriorSlowType2
// that sees as WarriorSlowType1Aggressive does.
func SlowType2Aggressive() script.Script {
	return script.Script{Dir: slowTypeDir, Behavior: true, NPCs: []int32{20996, 20458}, Hooks: slowType2AggressiveHooks()}
}

// slowType2AggressiveHooks are WarriorSlowType2Aggressive's hooks.
func slowType2AggressiveHooks() script.Hooks {
	base := slowType2Hooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			if n.LifeTime() > 7 && n.Intention() != script.IntentionAttack && n.InMyTerritory() {
				if n.Distance2D(c) > 100 && s.Rnd(100) < 10 {
					castByType(n, c, npc.SkillTypeDDMagicSlow)
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})
}

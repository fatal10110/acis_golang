package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// cast3Magical3Dir is the directory of WarriorCast3SkillsMagical3 and its
// children.
const cast3Magical3Dir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCast3SkillsMagical3"

// Cast3SkillsMagical3 is WarriorCast3SkillsMagical3: a Warrior that, hit by
// the player or summon it most wants to attack, may lay its debuff on it
// unless it already holds one and, from more than 100 away, blast it with
// its DD magic. Every hit by a player or summon also holds the attacker in
// place, once (IntAI0), as WarriorCastHoldMagic does, and arms the same
// 12 s timer. A settled clan member not attacking debuffs its clan's
// attacker unless the member itself holds the debuff, and blasts it from
// more than 100.
func Cast3SkillsMagical3() script.Script {
	return script.Script{Dir: cast3Magical3Dir, Behavior: true, NPCs: []int32{20611, 21025, 21115}, Hooks: Cast3SkillsMagical3Hooks()}
}

// Cast3SkillsMagical3Hooks are WarriorCast3SkillsMagical3's hooks, which
// its children derive theirs from.
func Cast3SkillsMagical3Hooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				topDesireTarget := n.TopDesireTarget()
				if topDesireTarget != nil && script.Same(topDesireTarget, attacker) {
					debuff, _ := n.SkillByType(npc.SkillTypeDebuff)
					if s.Rnd(100) < 33 && s.AbnormalLevel(attacker, debuff) <= 0 {
						castByType(n, attacker, npc.SkillTypeDebuff)
					}
				}
				dist := n.Distance2D(attacker)
				if dist > 100 {
					if topDesireTarget != nil && script.Same(topDesireTarget, attacker) && s.Rnd(100) < 33 {
						castByType(n, attacker, npc.SkillTypeDDMagic)
					}
				}
				s.StartTimer("2001", n, nil, 12000*time.Millisecond)
				if n.Int(script.SlotIntAI0) == 0 {
					i6 := s.Rnd(100)
					if dist > 300 {
						if i6 < 50 {
							castHold(n, attacker)
						}
					} else if dist > 100 {
						if topDesireTarget != nil && ((script.Same(topDesireTarget, attacker) && i6 < 50) || i6 < 10) {
							castHold(n, attacker)
						}
					}
					if script.AttackType(attacker) == item.WeaponBow && s.Rnd(100) < 50 {
						castHold(n, attacker)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack {
				debuff, _ := called.SkillByType(npc.SkillTypeDebuff)
				if s.Rnd(100) < 33 && s.AbnormalLevel(called, debuff) <= 0 {
					castByType(called, attacker, npc.SkillTypeDebuff)
				}
				if called.Distance2D(attacker) > 100 && s.Rnd(100) < 33 {
					castByType(called, attacker, npc.SkillTypeDDMagic)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if c0 := n.LastAttacker(); c0 != nil && n.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 80 {
					castHold(n, c0)
				}
			}
			return base.Timer(s, e)
		},
	}).Named("WarriorCast3SkillsMagical3")
}

// Cast3SkillsMagical3Aggressive is WarriorCast3SkillsMagical3Aggressive: a
// WarriorCast3SkillsMagical3 that attacks the players and summons it
// sees; settled, wandering in its territory, it may first lay its debuff
// on one holding none and blast one more than 100 away with its DD magic.
func Cast3SkillsMagical3Aggressive() script.Script {
	base := Cast3SkillsMagical3Hooks()
	return script.Script{Dir: cast3Magical3Dir, Behavior: true, NPCs: []int32{20821, 20628, 20856, 18007}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			if n.LifeTime() > 7 && n.Intention() == script.IntentionWander && n.InMyTerritory() {
				debuff, _ := n.SkillByType(npc.SkillTypeDebuff)
				if s.Rnd(100) < 33 && s.AbnormalLevel(c, debuff) <= 0 {
					castByType(n, c, npc.SkillTypeDebuff)
				}
				if n.Distance2D(c) > 100 && s.Rnd(100) < 33 {
					castByType(n, c, npc.SkillTypeDDMagic)
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	})}
}

// Cast3SkillsMagical3Revived is WarriorCast3SkillsMagical3Revived: a
// WarriorCast3SkillsMagical3 that, called with spawn parameters 1000 and
// a creature, stuns and attacks that creature as it appears.
func Cast3SkillsMagical3Revived() script.Script {
	base := Cast3SkillsMagical3Hooks()
	return script.Script{Dir: cast3Magical3Dir, Behavior: true, NPCs: []int32{21063}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if n.Int(script.SlotParam1) == 1000 {
				if c0 := s.Object(n.Int(script.SlotParam2)); c0 != nil {
					n.AddCastDesire(c0, skill.Ref{ID: 4663, Level: 1}, 10000)
					n.AddAttackDesire(c0, 500)
				}
			}
			base.Created(s, e)
		},
	})}
}

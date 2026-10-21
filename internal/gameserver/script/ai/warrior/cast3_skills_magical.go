package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// cast3MagicalDir is the directory of WarriorCast3SkillsMagical and its
// children.
const cast3MagicalDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCast3SkillsMagical"

// Cast3SkillsMagical is WarriorCast3SkillsMagical: a Warrior that, hit by
// a player or summon, puts to sleep a second attacker it does not hate
// most, blasts with its DD magic the one it hates most standing more than
// 100 away, and once may cancel one close by that holds none of its
// marks; it blasts a clan member's attacker while it is not attacking.
//
// Its memory: i_ai0 is set once a player or summon it does not hate most
// hit it; i_ai1 once it wanted its cancel.
func Cast3SkillsMagical() script.Script {
	return script.Script{Dir: cast3MagicalDir, Behavior: true, NPCs: []int32{20610, 20617, 21176, 21179, 21182, 21185, 20797, 21003, 20658, 20639, 20802, 20672}, Hooks: Cast3SkillsMagicalHooks()}
}

// Cast3SkillsMagicalHooks are WarriorCast3SkillsMagical's hooks, which its
// children derive theirs from.
func Cast3SkillsMagicalHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				mostHated := n.MostHated()
				if mostHated == nil {
					base.Attacked(s, e)
					return
				}
				switch {
				case !script.Same(mostHated, attacker):
					if n.Int(script.SlotIntAI0) == 0 {
						n.SetInt(script.SlotIntAI0, 1)
					} else if n.Int(script.SlotIntAI0) == 1 && s.Rnd(100) < 30 && n.HPRatio() > 0.1 {
						castByType(n, attacker, npc.SkillTypeSleepMagic)
					}
				case n.Distance2D(attacker) > 100:
					if s.Rnd(100) < 33 {
						castByType(n, attacker, npc.SkillTypeDDMagic)
					}
				case n.Int(script.SlotIntAI1) == 0 && s.Rnd(200) < 1 && n.HPRatio() > 0.06:
					if unmarked(s, n, attacker, npc.SkillTypeSleepMagic, npc.SkillTypeCheckMagic, npc.SkillTypeCheckMagic1, npc.SkillTypeCheckMagic2) {
						castByType(n, attacker, npc.SkillTypeCancelMagic)
						n.SetInt(script.SlotIntAI1, 1)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack && called.Distance2D(attacker) > 100 && s.Rnd(100) < 33 {
				castByType(called, attacker, npc.SkillTypeDDMagic)
			}
			base.ClanAttacked(s, e)
		},
	}).Named("WarriorCast3SkillsMagical")
}

// unmarked reports whether c holds no effect of the kind of any skill of
// the types n lists; a type n lists no skill for marks nothing.
func unmarked(s *script.Script, n *script.NPC, c script.Creature, types ...npc.SkillType) bool {
	for _, t := range types {
		if ref, ok := n.SkillByType(t); ok && s.AbnormalLevel(c, ref) > 0 {
			return false
		}
	}
	return true
}

// Cast3SkillsMagicalAggressive is WarriorCast3SkillsMagicalAggressive: a
// WarriorCast3SkillsMagical that attacks the players and summons it sees,
// out of a fight in its territory most likely blasting first with its DD
// magic one more than 100 away.
func Cast3SkillsMagicalAggressive() script.Script {
	return script.Script{Dir: cast3MagicalDir, Behavior: true, NPCs: []int32{20822, 20627, 20828, 20810, 20847, 21635, 21636, 21165, 21637, 18006}, Hooks: Cast3SkillsMagicalAggressiveHooks()}
}

// Cast3SkillsMagicalAggressiveHooks are WarriorCast3SkillsMagicalAggressive's
// hooks, which its children derive theirs from.
func Cast3SkillsMagicalAggressiveHooks() script.Hooks {
	base := Cast3SkillsMagicalHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				return
			}
			if n.LifeTime() > 7 && n.Intention() != script.IntentionAttack && n.InMyTerritory() {
				if n.Distance2D(c) > 100 && s.Rnd(100) < 33 {
					castByType(n, c, npc.SkillTypeDDMagic)
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
	}).Named("WarriorCast3SkillsMagicalAggressive")
}

// Cast3SkillsMagicalRevived is WarriorCast3SkillsMagicalRevived: a
// WarriorCast3SkillsMagical that, called with spawn parameters 1000 and a
// creature, stuns and attacks that creature as it appears.
func Cast3SkillsMagicalRevived() script.Script {
	base := Cast3SkillsMagicalHooks()
	return script.Script{Dir: cast3MagicalDir, Behavior: true, NPCs: []int32{20859, 21068}, Hooks: base.With(script.Hooks{
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

// Cast3SkillsCurse is WarriorCast3SkillsCurse: a WarriorCast3SkillsMagical
// that, hit twice within 5 seconds and then left alone for 5 more, lays its
// debuff on the creature it hates most; dying, it lifts that debuff from
// that creature.
//
// Its memory: i_ai3 is set by its first hit, which arms timer "5001"; i_ai4
// marks a hit since the timer was last armed; c_ai2 is the creature it
// last wanted to debuff, itself until one.
func Cast3SkillsCurse() script.Script {
	return script.Script{Dir: cast3MagicalDir, Behavior: true, NPCs: []int32{21301}, Hooks: Cast3SkillsCurseHooks()}
}

// Cast3SkillsCurseHooks are WarriorCast3SkillsCurse's hooks, which its
// children derive theirs from.
func Cast3SkillsCurseHooks() script.Hooks {
	base := Cast3SkillsMagicalHooks()
	return base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI3, 0)
			e.NPC.SetInt(script.SlotIntAI4, 0)
			e.NPC.SetSlot(script.SlotCreature2, e.NPC)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.Int(script.SlotIntAI4) == 0 {
				if n.Int(script.SlotIntAI3) == 0 {
					n.SetInt(script.SlotIntAI3, 1)
					s.StartTimer("5001", n, nil, 5000*time.Millisecond)
				} else if n.Int(script.SlotIntAI3) == 1 {
					n.SetInt(script.SlotIntAI4, 1)
				}
			}
			base.Attacked(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			if c := n.Slot(script.SlotCreature2); c != nil && !script.Same(c, n) {
				if cancel, ok := n.SkillByType(npc.SkillTypeDebuffCancel); ok {
					s.ApplyEffects(cancel, c, c)
				}
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "5001") && n.Int(script.SlotIntAI3) == 1 {
				switch n.Int(script.SlotIntAI4) {
				case 1:
					s.StartTimer("5001", n, nil, 5000*time.Millisecond)
					n.SetInt(script.SlotIntAI4, 0)
				case 0:
					if mostHated := n.MostHated(); mostHated != nil {
						castByType(n, mostHated, npc.SkillTypeDebuff)
						n.SetSlot(script.SlotCreature2, mostHated)
					}
				}
			}
			return base.Timer(s, e)
		},
	}).Named("WarriorCast3SkillsCurse")
}

// Cast3SkillsCurseAggressive is WarriorCast3SkillsCurseAggressive: a
// WarriorCast3SkillsCurse that attacks the players and summons it sees.
func Cast3SkillsCurseAggressive() script.Script {
	base := Cast3SkillsCurseHooks()
	return script.Script{Dir: cast3MagicalDir, Behavior: true, NPCs: []int32{21302}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	})}
}

// ZakenPrivateValeMaster is ZakenPrivateValeMaster: Zaken's Vale Master, a
// WarriorCast3SkillsMagicalAggressive that keeps where it was created, only
// notices the players and summons within 100 of its height, attacking
// them in its territory, and curses an attacker, and mutes a caster, more
// than 8 levels above it.
//
// Its memory: WeightPoint, RespawnTime and Flag hold the x, y and z it was
// created at.
func ZakenPrivateValeMaster() script.Script {
	base := Cast3SkillsMagicalAggressiveHooks()
	return script.Script{Dir: cast3MagicalDir, Behavior: true, NPCs: []int32{29024}, Hooks: base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			at := script.LocOf(e.NPC)
			e.NPC.SetInt(script.SlotWeightPoint, int32(at.X))
			e.NPC.SetInt(npc.IntRespawnTime, int32(at.Y))
			e.NPC.SetInt(script.SlotFlag, int32(at.Z))
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			z, own := script.LocOf(c).Z, script.LocOf(n).Z
			if z > own-100 && z < own+100 {
				if !c.Playable() {
					return
				}
				if n.InMyTerritory() {
					n.AddAttackDesire(c, 200)
				}
				defaultnpc.MakeAttackEvent(s, n, c, 9, true)
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if e.Attacker.Level() > e.NPC.Level()+8 {
				e.NPC.AddCastDesire(e.Attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)
			}
			base.Attacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if e.Caster.Level() > e.NPC.Level()+8 {
				e.NPC.AddCastDesire(e.Caster, skill.Ref{ID: 4215, Level: 1}, 1000000)
				return
			}
			base.SeeSpell(s, e)
		},
	})}
}

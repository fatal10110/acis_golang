package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// CastSelfRangeDDMagicAggressive is WarriorCastSelfRangeDDMagicAggressive:
// an aggressive Warrior that strikes from afar with its long range magic,
// at a player or summon it sees while it fights no one and at the creature
// it fights while that one stands between 200 and 600 away, and that may
// strike around itself with its self range magic at an attacker other than
// the one it fights, or, for its clan, at itself. Hit while it cannot reach
// the creature it fights, it turns on the attacker or strikes that one
// from afar.
func CastSelfRangeDDMagicAggressive() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{21310, 21381, 21387, 21401}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				return
			}
			if n.LifeTime() > 7 && n.InMyTerritory() && n.TopDesireTarget() == nil {
				if n.Distance2D(c) > 200 {
					addCastByType(n, c, npc.SkillTypeWLongRangeDDMagic, 1000000)
				}
				defaultnpc.TryToAttack(s, n, c)
				base.SeeCreature(s, e)
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker, damage := e.NPC, e.Attacker, float64(e.Damage)
			if attacker.Playable() {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil {
					dist := n.Distance2D(topDesireTarget)
					if dist > 40 && s.AbnormalLevel(n, skill.Ref{ID: 1201, Level: 1}) >= 0 {
						if n.CanAttack(topDesireTarget) {
							n.AddAttackDesire(attacker, damageHate(n, damage, defaultnpc.HateRatio(n, attacker))*100)
						} else if !n.CanAttack(topDesireTarget) && dist < 600 {
							addCastByType(n, topDesireTarget, npc.SkillTypeWLongRangeDDMagic, 1000000)
						} else {
							n.StopHate(topDesireTarget)
							n.AddAttackDesire(attacker, damageHate(n, damage, defaultnpc.HateRatio(n, attacker))*100)
						}
					}
					dist = n.Distance2D(attacker)
					if dist > 200 && dist < 600 && script.Same(topDesireTarget, attacker) {
						addCastByType(n, attacker, npc.SkillTypeWLongRangeDDMagic, 1000000)
					}
					if dist < 200 && !script.Same(topDesireTarget, attacker) && s.Rnd(100) < 33 {
						addCastByType(n, attacker, npc.SkillTypeSelfRangeDDMagic, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 {
				if topDesireTarget := called.TopDesireTarget(); topDesireTarget != nil && !script.Same(topDesireTarget, attacker) {
					if called.Distance2D(attacker) < 200 && s.Rnd(100) < 33 {
						addCastByType(called, called, npc.SkillTypeSelfRangeDDMagic, 1000000)
					}
				}
			}
			base.ClanAttacked(s, e)
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if topDesireTarget := n.TopDesireTarget(); isPlayer(topDesireTarget) {
				if n.Distance2D(topDesireTarget) > 200 {
					addCastByType(n, topDesireTarget, npc.SkillTypeWLongRangeDDMagic, 1000000)
				}
			}
			base.UseSkillFinished(s, e)
		},
	})}
}

// CastSelfRangeDDMagicBuffAggressive is
// WarriorCastSelfRangeDDMagicBuffAggressive: an aggressive Warrior that
// buffs itself as it appears and, while it neither attacks nor casts,
// whenever it sees a creature or a player or summon hits it and the buff
// is gone, buffs the clan member it helps the same way, and may strike
// around itself with its self range magic when an attacker other than the
// one it fights comes within 200.
func CastSelfRangeDDMagicBuffAggressive() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{21390, 21391}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			addCastByType(e.NPC, e.NPC, npc.SkillTypeBuff, 1000000)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !fighting(n.Intention()) {
				rebuff(s, n, n)
			}
			if !c.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if !fighting(n.Intention()) {
					rebuff(s, n, n)
				}
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil && !script.Same(topDesireTarget, attacker) {
					if s.Rnd(100) < 33 && n.Distance2D(attacker) < 200 {
						addCastByType(n, n, npc.SkillTypeSelfRangeDDMagic, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called, attacker := e.Caller, e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 {
				if topDesireTarget := called.TopDesireTarget(); topDesireTarget != nil && !script.Same(topDesireTarget, attacker) {
					if called.Distance2D(attacker) < 200 && s.Rnd(100) < 33 {
						addCastByType(called, called, npc.SkillTypeSelfRangeDDMagic, 1000000)
					}
				}
				if !fighting(called.Intention()) {
					rebuff(s, called, caller)
				}
			}
			base.ClanAttacked(s, e)
		},
	})}
}

// rebuff makes n want to cast its template's buff on target, with weight
// 1000000, when target holds no effect of the buff's stack; a template
// that lists no buff casts nothing.
func rebuff(s *script.Script, n *script.NPC, target script.Creature) {
	buff, ok := n.SkillByType(npc.SkillTypeBuff)
	if s.AbnormalLevel(target, buff) <= 0 && ok {
		n.AddCastDesire(target, buff, 1000000)
	}
}

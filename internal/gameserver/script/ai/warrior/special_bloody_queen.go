package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// SpecialBloodyQueen is SpecialBloodyQueen: a
// WarriorCastingCurseAggressive that, once in its life (i_ai0), may cast
// its DD magic on a player or summon that hits it, the more likely the
// farther it stands and the more so when it is the creature the queen
// desires most; on a clan member's attacker standing beyond 100; and,
// settled in its territory, on a creature it sees beyond 100.
func SpecialBloodyQueen() script.Script {
	base := CastingCurseAggressiveHooks()
	return script.Script{Dir: castingCurseAggressiveDir, Behavior: true, NPCs: []int32{21084}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() && n.Int(script.SlotIntAI0) == 0 {
				i1 := s.Rnd(100)
				switch dist := n.Distance2D(attacker); {
				case dist > 300:
					if i1 < 80 {
						castDDMagicOnce(n, attacker)
					}
				case dist > 100:
					if top := n.TopDesireTarget(); top != nil && (script.Same(top, attacker) && i1 < 70 || i1 < 30) {
						castDDMagicOnce(n, attacker)
					}
				case i1 < 10:
					if top := n.TopDesireTarget(); top != nil && script.Same(top, attacker) {
						castDDMagicOnce(n, attacker)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 {
				if called.Distance2D(attacker) > 100 && called.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 40 {
					castDDMagicOnce(called, attacker)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if n.LifeTime() > 7 && n.InMyTerritory() {
				if n.Distance2D(c) > 100 && n.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 20 {
					castDDMagicOnce(n, c)
				}
			}
			base.SeeCreature(s, e)
		},
	})}
}

// castDDMagicOnce makes n want to cast its DD magic on target and spends
// its one cast (i_ai0), even when its template lists no DD magic.
func castDDMagicOnce(n *script.NPC, target script.Creature) {
	addCastByType(n, target, npc.SkillTypeDDMagic, 1000000)
	n.SetInt(script.SlotIntAI0, 1)
}

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

const castHoldMagicDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorCastHoldMagic"

// CastHoldMagic is WarriorCastHoldMagic: a Warrior that holds a player or
// summon hitting it in place, once (IntAI0): on a draw under 50 from
// farther than 300; from farther than 100, on a draw under 50 when it is
// the one it most wants to attack, else under 10; and, on a second draw
// under 50, when it attacks with a bow. Each such hit also starts a 12 s
// timer that, unless it already held someone, holds its last attacker on
// a draw under 80.
func CastHoldMagic() script.Script {
	return script.Script{Dir: castHoldMagicDir, Behavior: true, NPCs: []int32{21628, 27118, 20663, 20844, 21626, 21627}, Hooks: castHoldMagicHooks()}
}

// castHold makes n want to cast its hold magic on target, weight 1000000,
// and marks IntAI0 that it did.
func castHold(n *script.NPC, target script.Creature) {
	addCastByType(n, target, npc.SkillTypeHoldMagic, 1000000)
	n.SetInt(script.SlotIntAI0, 1)
}

// castHoldMagicHooks are WarriorCastHoldMagic's hooks, which its children
// derive theirs from.
func castHoldMagicHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				i6 := s.Rnd(100)
				s.StartTimer("2001", n, nil, 12000*time.Millisecond)
				if n.Int(script.SlotIntAI0) == 0 {
					if distance := n.Distance2D(attacker); distance > 300 {
						if i6 < 50 {
							castHold(n, attacker)
						}
					} else if distance > 100 {
						if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil && ((script.Same(topDesireTarget, attacker) && i6 < 50) || i6 < 10) {
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
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if c0 := n.LastAttacker(); c0 != nil && n.Int(script.SlotIntAI0) == 0 && s.Rnd(100) < 80 {
					castHold(n, c0)
				}
			}
			return base.Timer(s, e)
		},
	}).Named("WarriorCastHoldMagic")
}

// CastHoldMagicAggressive is WarriorCastHoldMagicAggressive: a
// WarriorCastHoldMagic that attacks the players and summons it sees.
func CastHoldMagicAggressive() script.Script {
	base := castHoldMagicHooks()
	return script.Script{Dir: castHoldMagicDir, Behavior: true, NPCs: []int32{20134, 20105, 20144, 20057, 20171, 20351, 20419, 18008}, Hooks: base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	})}
}

// OrfenPrivateRiba is OrfenPrivateRiba: Orfen's Riba, a
// WarriorCastHoldMagic that every 90 to 330 s may shuffle its hate. It
// curses an attacker more than 8 levels above it and takes no other hit
// to heart, and mutes such a caster.
func OrfenPrivateRiba() script.Script {
	base := castHoldMagicHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior/WarriorCastHoldMagicAggressive", Behavior: true, NPCs: []int32{29017}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimer("5001", e.NPC, nil, coreShuffleDelay(s))
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "5001") {
				if s.Rnd(100) < 66 {
					e.NPC.RandomizeAttack()
				}
				s.StartTimer("5001", e.NPC, nil, coreShuffleDelay(s))
			}
			return base.Timer(s, script.Timer{Name: e.Name, NPC: e.NPC})
		},
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			if e.Attacker.Level() > e.NPC.Level()+8 {
				e.NPC.AddCastDesire(e.Attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)
			}
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

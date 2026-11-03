package monster

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// ddHoldRange is how far a WarriorDDMagicHold keeps its top desire target
// before it lets it go.
const ddHoldRange = 500

// WarriorDDMagicHold is a monster that holds its ground and strikes with
// its magic: a playable that hits it or its clan draws its magic strike,
// held, unless it is already fighting, and its held attack; a top desire
// target farther than 500 is let go. It binds no NPC of its own.
func WarriorDDMagicHold() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorDDMagicHold", Behavior: true, Hooks: warriorDDMagicHoldHooks()}
}

// fighting reports whether n attacks or casts.
func fighting(n *script.NPC) bool {
	i := n.Intention()
	return i == script.IntentionAttack || i == script.IntentionCast
}

// holdDDMagic has n want its magic strike on target, held, at 1000000;
// nothing when its template lists none.
func holdDDMagic(n *script.NPC, target script.Creature) {
	if ref, ok := n.SkillByType(npc.SkillTypeDDMagic); ok {
		n.AddCastDesireHold(target, ref, 1000000)
	}
}

// warriorDDMagicHoldHooks are WarriorDDMagicHold's hooks, which the
// aggressive kind derives from.
func warriorDDMagicHoldHooks() script.Hooks {
	base := Hooks()
	return base.With(script.Hooks{
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if !attacker.Playable() {
				return
			}
			if !fighting(n) {
				holdDDMagic(n, attacker)
			} else if top := n.TopDesireTarget(); top != nil && !script.Same(attacker, top) && n.Distance3D(top) > ddHoldRange {
				n.StopHate(top)
				holdDDMagic(n, attacker)
			}
			n.AddAttackDesireHold(attacker, 100)
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			called := e.Called
			if e.Attacker.Playable() && called.LifeTime() > 7 {
				if !fighting(called) {
					holdDDMagic(called, e.Attacker)
				}
				called.AddAttackDesireHold(e.Attacker, 50)
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			top := n.TopDesireTarget()
			if top == nil {
				return
			}
			if ddHoldRange < n.Distance3D(top) {
				n.StopHate(top)
				s.StartTimer("2001", n, nil, 1000*time.Millisecond)
				return
			}
			if _, ok := top.(*script.Player); ok {
				holdDDMagic(n, top)
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "2001") {
				n := e.NPC
				if top := n.TopDesireTarget(); top != nil {
					if ddHoldRange < n.Distance3D(top) {
						n.StopHate(top)
						s.StartTimer("2001", n, e.Player, 1000*time.Millisecond)
						return base.Timer(s, e)
					}
					if _, ok := top.(*script.Player); ok {
						holdDDMagic(n, top)
					}
				}
			}
			return base.Timer(s, e)
		},
	}).Named("WarriorDDMagicHold")
}

// WarriorDDMagicHoldAggressive is a WarriorDDMagicHold that goes for what
// it sees: its magic strike, held, unless it is already fighting, and its
// held attack on a playable. It binds no NPC of its own.
func WarriorDDMagicHoldAggressive() script.Script {
	return script.Script{Dir: ddHoldAggressiveDir, Behavior: true, Hooks: warriorDDMagicHoldAggressiveHooks()}
}

// ddHoldAggressiveDir is where the aggressive kind and its derivatives live.
const ddHoldAggressiveDir = "ai/individual/Monster/WarriorDDMagicHold/WarriorDDMagicHoldAggressive"

// warriorDDMagicHoldAggressiveHooks are WarriorDDMagicHoldAggressive's
// hooks, which Valakas's lavasaurus derives from.
func warriorDDMagicHoldAggressiveHooks() script.Hooks {
	base := warriorDDMagicHoldHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(_ *script.Script, e script.SeeCreature) {
			n := e.NPC
			if !fighting(n) {
				holdDDMagic(n, e.Creature)
			}
			if e.Creature.Playable() {
				n.AddAttackDesireHold(e.Creature, 50)
			}
		},
	}).Named("WarriorDDMagicHoldAggressive")
}

// ValakasPrivateLavasaurus is the lavasaurus Valakas raises: it strikes
// the creature its first spawn parameter names with its magic, held, and
// every minute it is not fighting it leaves the world.
func ValakasPrivateLavasaurus() script.Script {
	base := warriorDDMagicHoldAggressiveHooks()
	return script.Script{Dir: ddHoldAggressiveDir, Behavior: true, NPCs: []int32{29029}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if c0 := s.Object(n.Int(script.SlotParam1)); c0 != nil {
				holdDDMagic(n, c0)
			}
			s.StartTimer("3001", n, nil, 60000*time.Millisecond)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3001") {
				n := e.NPC
				if !fighting(n) {
					n.DeleteMe()
				} else {
					s.StartTimer("3001", n, nil, 60000*time.Millisecond)
				}
			}
			return base.Timer(s, e)
		},
	}).Named("ValakasPrivateLavasaurus")}
}

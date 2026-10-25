package agitwarrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// WarriorFlee is AgitWarriorFlee: an AgitWarrior that breaks off once worn
// down. Its second int slot is 1 while it flees: then each hit makes it
// want to run its drift range from the attacker, weight 10000, and it
// ignores what it sees.
func WarriorFlee() script.Script {
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35429, 35619}, Hooks: WarriorFleeHooks()}
}

// WarriorFleeHooks are AgitWarriorFlee's hooks, which its children derive
// theirs from. Created, the NPC fights; timer 3001 makes it fight again.
func WarriorFleeHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3001") {
				e.NPC.SetInt(script.SlotIntAI1, 0)
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if e.NPC.Int(script.SlotIntAI1) == 1 {
				e.NPC.AddFleeDesire(e.Attacker, e.NPC.MaxDriftRange(), 10000)
			} else {
				fightOrBreak(s, e)
			}
		},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)

			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if e.NPC.Int(script.SlotIntAI1) == 0 {
				base.SeeCreature(s, e)
			}
		},
	}).Named("AgitWarriorFlee")
}

// Thief is AgitThief: an AgitWarriorFlee that drops every attack desire
// at each hit while it flees and, on a draw under 10 of 100, wants to hold
// the attacker.
func Thief() script.Script {
	base := WarriorFleeHooks()
	return script.Script{Dir: fleeDir, Behavior: true, NPCs: []int32{35432, 35622}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.Int(script.SlotIntAI1) == 1 {
				n.RemoveAllAttackDesire()
				n.AddFleeDesire(e.Attacker, n.MaxDriftRange(), 10000)

				if s.Rnd(100) < 10 {
					n.AddCastDesire(e.Attacker, skill.Ref{ID: 4047, Level: 6}, 1000000)
				}
			} else {
				fightOrBreak(s, e)
			}
		},
	})}
}

// WizardFlee is AgitWizardFlee: an AgitWarriorFlee that, on a draw under
// 10 of 100 at each hit while it flees, wants to heal itself.
func WizardFlee() script.Script {
	base := WarriorFleeHooks()
	return script.Script{Dir: fleeDir, Behavior: true, NPCs: []int32{35431, 35621}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if n.Int(script.SlotIntAI1) == 1 {
				n.AddFleeDesire(e.Attacker, n.MaxDriftRange(), 10000)

				if s.Rnd(100) < 10 {
					n.AddCastDesire(n, skill.Ref{ID: 4065, Level: 6}, 1000000)
				}
			} else {
				fightOrBreak(s, e)
			}
		},
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI1, 0)

			base.Created(s, e)
		},
	})}
}

// fightOrBreak answers the hit e on a fighting AgitWarriorFlee, when a
// player of another clan, or a clanless one or its summon, landed it:
// above half its hit points the NPC wants to attack the attacker with its
// damage weight, cut to a whole number; above 30% it does so on a draw
// under 90 of 100 and else starts to flee, dropping every attack desire,
// until timer 3001 ends the flight 5 s later; at 30% or less it starts to
// flee for good.
func fightOrBreak(s *script.Script, e script.Attacked) {
	n := e.NPC
	p := script.ActingPlayer(e.Attacker)
	if p == nil || !outsider(n, p) {
		return
	}
	switch hpRatio := n.HPRatio(); {
	case hpRatio > 0.5:
		n.AddAttackDesire(e.Attacker, float64(int(damageWeight(e))))
	case hpRatio > 0.3:
		if s.Rnd(100) < 90 {
			n.AddAttackDesire(e.Attacker, float64(int(damageWeight(e))))
		} else {
			n.SetInt(script.SlotIntAI1, 1)
			n.RemoveAllAttackDesire()

			s.StartTimer("3001", n, p, 5000*time.Millisecond)
		}
	default:
		n.SetInt(script.SlotIntAI1, 1)
		n.RemoveAllAttackDesire()
	}
}

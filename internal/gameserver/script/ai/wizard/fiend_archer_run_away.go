package wizard

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// FiendArcherRunAway is WizardFiendArcherRunAway: a Wizard that shoots its
// fiend archer skill at a playable that hits it and, after each cast, at
// its most hated, stepping away from one closer than 100 first. Hit below
// half its HP, it may run, on a draw under 10, to the flee point its spawn
// names (int slot 3 set to 1). There it wanders and, 1 s later, either
// turns on the creature it last fought and calls its kin within 400 to it
// (script event 10000), or waits to heal fully and walks home. A rooted
// runner held farther than 40 from its target gives up the run. A settled
// one with an empty hate list answers a playable's hit on its clan member
// with its fiend archer skill. Short of MP, it attacks instead.
func FiendArcherRunAway() script.Script {
	base := WizardHooks()
	return script.Script{Dir: wizardDir, Behavior: true, NPCs: []int32{21514, 21509}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI3, 0)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil {
				n.SetSlot(script.SlotCreature0, topDesireTarget)
				if n.Rooted() && n.Distance2D(topDesireTarget) > 40 && n.Int(script.SlotIntAI3) == 1 {
					n.SetInt(script.SlotIntAI3, 3)
					n.RemoveAllDesire()
				}
			} else {
				n.SetSlot(script.SlotCreature0, attacker)
			}
			if fleeLoc, ok := n.FleeLocation(); ok && n.Int(script.SlotIntAI3) == 0 && n.HPRatio() < 0.5 && s.Rnd(100) < 10 {
				n.SetInt(script.SlotIntAI3, 1)
				n.RemoveAllDesire()
				n.AddMoveToDesire(fleeLoc, 2000)
				return
			}
			if attacker.Playable() {
				f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
				hateList := n.HateList()
				if hateList.Empty() {
					hateList.Add(attacker, (f0*100)+300)
				} else {
					hateList.Add(attacker, f0*100)
				}
				if !hateList.Empty() {
					castOrAttack(n, attacker, npc.SkillTypeWFiendArcher, true)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			hateList := called.HateList()
			hateList.Refresh()
			if attacker.Playable() && called.LifeTime() > 7 && hateList.Empty() {
				castOrAttack(called, attacker, npc.SkillTypeWFiendArcher, true)
			}
			base.ClanAttacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "2001"):
				if n.Int(script.SlotIntAI3) == 2 && n.Intention() != script.IntentionAttack {
					if c := n.Slot(script.SlotCreature0); c != nil {
						n.AddAttackDesire(c, 1000)
						s.BroadcastScriptEvent(n, 10000, c.ObjectID(), 400)
					}
					n.SetInt(script.SlotIntAI3, 3)
				}
			case strings.EqualFold(e.Name, "2002"):
				if n.Int(script.SlotIntAI0) == 2 {
					if float64(n.MaxHP()) == n.HP() {
						n.AddMoveToDesire(n.SpawnLocation(), 5000)
					} else {
						s.StartTimer("2002", n, nil, 1000*time.Millisecond)
					}
				}
			}
			return base.Timer(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil {
				if n.Distance2D(mostHatedHI) < 100 {
					n.AddFleeDesire(mostHatedHI, n.MaxDriftRange(), 1000)
				}
				castOrAttack(n, mostHatedHI, npc.SkillTypeWFiendArcher, true)
			}
		},
		OnMoveToFinished: func(s *script.Script, e script.MoveToFinished) {
			n := e.NPC
			switch n.Int(script.SlotIntAI3) {
			case 1:
				if fleeLoc, ok := n.FleeLocation(); ok && atLoc(fleeLoc, e) {
					n.ClearDesires()
					n.AddWanderDesire(5, 50)
					n.SetInt(script.SlotIntAI3, 2)
					if s.Rnd(100) < 50 {
						s.StartTimer("2001", n, nil, 1000*time.Millisecond)
					} else {
						s.StartTimer("2002", n, nil, 1000*time.Millisecond)
					}
				}
			case 3:
				if atLoc(n.SpawnLocation(), e) {
					n.SetInt(script.SlotIntAI3, 0)
				}
			default:
				if mostHatedHI := n.HateList().MostHated(); mostHatedHI != nil {
					castOrAttack(n, mostHatedHI, npc.SkillTypeWFiendArcher, true)
				}
			}
		},
	})}
}

// atLoc reports whether the arrival of e is exactly at l.
func atLoc(l script.Loc, e script.MoveToFinished) bool {
	return l.X == int(e.X) && l.Y == int(e.Y) && l.Z == int(e.Z)
}

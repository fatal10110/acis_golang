package wizard

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Wizard is the Wizard behavior: a monster that wanders, plays its social
// animations and shouts its lines, and fills its hate list by the damage
// dealt to it. Its hate list forgets, every 10 s, the creatures it lost
// and the ones 1000 or more away. A silenced wizard waits for the silence
// to end, then answers its most hated as if hit by it. It binds no NPC of
// its own.
func Wizard() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WizardBase/Wizard", Behavior: true, Hooks: WizardHooks()}
}

// WizardHooks are Wizard's hooks, which its children derive theirs from.
func WizardHooks() script.Hooks {
	base := BaseHooks()
	return base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddWanderDesire(5, 5)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if shoutMsg1 := n.IntParam("ShoutMsg1"); shoutMsg1 > 0 {
				n.Shout(script.NpcString(shoutMsg1))
			}
			if n.IntParam("MoveAroundSocial") > 0 || n.IntParam("ShoutMsg2") > 0 || n.IntParam("ShoutMsg3") > 0 {
				s.StartTimerAtFixedRate("1001", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			}
			s.StartTimerAtFixedRate("1002", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			n.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			hateList := n.HateList()
			switch {
			case strings.EqualFold(e.Name, "1001"):
				switch n.Intention() {
				case script.IntentionIdle, script.IntentionWander:
					moveAroundSocial2 := n.IntParam("MoveAroundSocial2")
					moveAroundSocial1 := n.IntParam("MoveAroundSocial1")
					moveAroundSocial := n.IntParam("MoveAroundSocial")
					if moveAroundSocial2 > 0 && s.Rnd(100) < 20 {
						n.AddSocialDesire(3, int(moveAroundSocial2*1000/30), 50)
					} else if moveAroundSocial1 > 0 && s.Rnd(100) < 20 {
						n.AddSocialDesire(2, int(moveAroundSocial1*1000/30), 50)
					} else if moveAroundSocial > 0 && s.Rnd(100) < 20 {
						n.AddSocialDesire(1, int(moveAroundSocial*1000/30), 50)
					}
				case script.IntentionAttack:
					if shoutMsg3 := n.IntParam("ShoutMsg3"); shoutMsg3 > 0 && s.Rnd(100) < 10 {
						n.Shout(script.NpcString(shoutMsg3))
					}
				}
			case strings.EqualFold(e.Name, "1002"):
				hateList.Refresh()
				hateList.RemoveIfOutOfRange(1000)
			case strings.EqualFold(e.Name, "1003"):
				if n.Muted() {
					s.StartTimer("1003", n, nil, 10000*time.Millisecond)
				} else {
					n.RemoveAllAttackDesire()
					n.SetInt(script.SlotIntAI0, 0)
					if mostHated := hateList.MostHated(); mostHated != nil {
						s.Attacked(s, script.Attacked{NPC: n, Attacker: mostHated, Damage: 100})
					}
				}
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				hateList := n.HateList()
				f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
				if hateList.Empty() {
					hateList.Add(attacker, (f0*100)+300)
				} else {
					hateList.Add(attacker, f0*100)
				}
			}
			if n.Muted() {
				n.SetInt(script.SlotIntAI0, 1)
				s.StartTimer("1003", n, nil, 10000*time.Millisecond)
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 {
				hateList := called.HateList()
				f0 := damageHate(called, float64(e.Damage), defaultnpc.HateRatio(called, attacker))
				if hateList.Empty() {
					hateList.Add(attacker, (f0*30)+300)
				} else {
					hateList.Add(attacker, f0*30)
				}
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			k := s.Skill(e.Skill)
			if k.AggroPoints() > 0 && !k.Offensive() {
				hateList := n.HateList()
				f0 := damageHate(n, float64(k.AggroPoints()), defaultnpc.HateRatio(n, caster))
				mostHated := n.MostHated()
				if n.Intention() == script.IntentionAttack && mostHated != nil && script.Same(mostHated, caster) {
					hateList.Add(caster, f0*150)
				} else {
					hateList.Add(caster, f0*75)
				}
			}
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			if shoutMsg4 := e.NPC.IntParam("ShoutMsg4"); shoutMsg4 > 0 && s.Rnd(100) < 30 {
				e.NPC.Shout(script.NpcString(shoutMsg4))
			}
		},
	}).Named("Wizard")
}

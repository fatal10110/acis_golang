package wizard

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// partyLeaderDir is the directory the party leader wizards are listed
// under.
const partyLeaderDir = "ai/individual/Monster/WizardBase/PartyLeaderWizard"

// PartyLeader is PartyLeaderWizard: a WizardBase that leads its privates.
// It creates them when it spawns, or on a hit at its SummonPrivateRate
// chance, fills its hate list from every hit on itself and its party, and
// brings a dead private back after its respawn delay. Silenced, it waits
// for the silence to end and then answers the creature its hate list holds
// first. It binds no NPC of its own.
func PartyLeader() script.Script {
	return script.Script{Dir: partyLeaderDir, Behavior: true, Hooks: partyLeaderHooks()}
}

// partyLeaderHooks are PartyLeaderWizard's hooks, which its children derive
// theirs from.
func partyLeaderHooks() script.Hooks {
	base := BaseHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			s.StartTimerAtFixedRate("1002", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			n.SetInt(script.SlotWeightPoint, 10)
			n.SetInt(script.SlotIntAI0, 0)
			if n.IntParam("SummonPrivateRate") == 0 {
				defaultnpc.CreatePrivates(s, n)
				n.SetInt(script.SlotIntAI2, 1)
			} else {
				n.SetInt(script.SlotIntAI2, 0)
			}
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			hateList := n.HateList()
			if strings.EqualFold(e.Name, "1002") {
				hateList.Refresh()
				hateList.RemoveIfOutOfRange(1000)
			} else if strings.EqualFold(e.Name, "1003") {
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
			hateList := n.HateList()
			if attacker.Playable() {
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
			if n.Int(script.SlotIntAI2) == 0 {
				if int32(s.Rnd(100)) < n.IntParam("SummonPrivateRate") {
					defaultnpc.CreatePrivates(s, n)
					switch s.Rnd(4) {
					case 0:
						n.Say(script.NpcString(1000294))
					case 1:
						n.Say(script.NpcString(1000403))
					case 2:
						n.Say(script.NpcString(1000404))
					case 3:
						n.Say(script.NpcString(1000405))
					}
					n.SetInt(script.SlotIntAI2, 1)
				}
			}
			if isPlayer(attacker) && n.IntParam("ShoutTarget") == 1 {
				if s.Rnd(100) < 50 && attacker.HPRatio() < 0.4 {
					if script.Same(hateList.MostHated(), attacker) {
						switch s.Rnd(3) {
						case 0:
							n.Say(script.NpcString(1000291, attacker.Name()))
						case 1:
							n.Say(script.NpcString(1000398, attacker.Name()))
						case 2:
							n.Say(script.NpcString(1000399, attacker.Name()))
						}
						hateList.Clear()
						f0 := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
						hateList.Add(attacker, (f0*1000)+300)
						n.SetInt(script.SlotFlag, attacker.ObjectID())
						s.BroadcastScriptEvent(n, 10002, n.ObjectID(), 300)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(_ *script.Script, e script.PartyAttacked) {
			caller, called, target := e.Caller, e.Called, e.Target
			if target.Playable() && !script.Same(caller, called) {
				hateList := called.HateList()
				f0 := damageHate(called, float64(e.Damage), defaultnpc.HateRatio(called, target))
				weightPoint := float64(caller.Int(script.SlotWeightPoint))
				if hateList.Empty() {
					hateList.Add(target, (((f0*float64(e.Damage))*weightPoint)*10)+300)
				} else {
					hateList.Add(target, ((f0*float64(e.Damage))*weightPoint)*10)
				}
			}
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 {
				hateList := called.HateList()
				f0 := defaultnpc.HateRatio(called, attacker)
				if hateList.Empty() {
					hateList.Add(attacker, (((f0*float64(e.Damage))*30)*10)+300)
				} else {
					hateList.Add(attacker, ((f0*float64(e.Damage))*30)*10)
				}
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			k := s.Skill(e.Skill)
			if effectPoints := k.AggroPoints(); effectPoints > 0 && !k.Offensive() {
				hateList := n.HateList()
				f0 := damageHate(n, float64(effectPoints), defaultnpc.HateRatio(n, caster))
				if mostHated := n.MostHated(); n.Intention() == script.IntentionAttack && mostHated != nil && script.Same(mostHated, caster) {
					hateList.Add(caster, f0*150)
				} else {
					hateList.Add(caster, f0*75)
				}
			}
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			if shoutMsg4 := n.IntParam("ShoutMsg4"); shoutMsg4 > 0 && s.Rnd(100) < 30 {
				n.Shout(script.NpcString(shoutMsg4))
			}
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			if delay := s.Spawner().RespawnDelay(caller); delay != 0 && called.IsMaster() && !called.Dead() {
				s.Spawner().ScheduleRespawn(caller, time.Duration(delay)*1000*time.Millisecond)
			}
		},
	}).Named("PartyLeaderWizard")
}

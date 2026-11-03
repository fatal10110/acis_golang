package wizard

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// partyPrivateDir is the directory the party private wizards are listed
// under.
const partyPrivateDir = "ai/individual/Monster/WizardBase/PartyPrivateWizard"

// eventMasterTarget is the script event a party leader broadcasts, with its
// own object id, when it shouts the target its party should turn on.
const eventMasterTarget = 10002

// PartyPrivate is PartyPrivateWizard: a WizardBase that follows its master
// and fills its hate list from every hit on itself and its party. It turns
// on the target its master shouts, answers its clan and the buffs it sees
// only once its master is dead, and then leaves the world at a 10 s check
// that finds it neither attacking nor casting. Silenced, it waits for the
// silence to end and then answers the creature its hate list holds first.
// It binds no NPC of its own.
func PartyPrivate() script.Script {
	return script.Script{Dir: partyPrivateDir, Behavior: true, Hooks: partyPrivateHooks()}
}

// deadMaster reports whether n has a master and that master is dead.
func deadMaster(n *script.NPC) bool {
	return n.HasMaster() && n.Master().Dead()
}

// partyPrivateHooks are PartyPrivateWizard's hooks, which its children
// derive theirs from.
func partyPrivateHooks() script.Hooks {
	base := BaseHooks()
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			s.StartTimerAtFixedRate("1002", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			s.StartTimerAtFixedRate("1006", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
			n.SetInt(script.SlotIntAI0, 0)
			base.Created(s, e)
		},
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			n := e.NPC
			if master := n.Master(); master != nil && !master.Dead() {
				n.AddFollowDesire(master, 50)
			} else {
				n.AddWanderDesire(5, 5)
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			hateList := n.HateList()
			switch {
			case strings.EqualFold(e.Name, "1002"):
				hateList.Refresh()
				hateList.RemoveIfOutOfRange(1000)
			case strings.EqualFold(e.Name, "1003"):
				if n.Muted() {
					s.StartTimer("1003", n, e.Player, 10000*time.Millisecond)
				} else {
					n.RemoveAllAttackDesire()
					n.SetInt(script.SlotIntAI0, 0)
					if mostHated := hateList.MostHated(); mostHated != nil {
						s.Attacked(s, script.Attacked{NPC: n, Attacker: mostHated, Damage: 100})
					}
				}
			case strings.EqualFold(e.Name, "1006"):
				if deadMaster(n) && !fighting(n.Intention()) {
					s.Spawner().DecayMe(n)
					return ""
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
			base.Attacked(s, e)
		},
		OnPartyAttacked: func(_ *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			if target.Playable() {
				hateList := called.HateList()
				f0 := damageHate(called, float64(e.Damage), defaultnpc.HateRatio(called, target))
				if hateList.Empty() {
					hateList.Add(target, (f0*10)+300)
				} else {
					hateList.Add(target, f0*10)
				}
			}
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && deadMaster(called) && called.LifeTime() > 7 {
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
			if !deadMaster(n) {
				return
			}
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
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if n.Dead() || e.EventID != eventMasterTarget {
				return
			}
			master := n.Master()
			if master == nil || e.Arg1 != master.ObjectID() {
				return
			}
			flagged := s.Object(master.Int(script.SlotFlag))
			if top := n.TopDesireTarget(); top != nil && script.Same(top, flagged) {
				return
			}
			switch s.Rnd(4) {
			case 0:
				n.Say(script.NpcString(1000292))
			case 1:
				n.Say(script.NpcString(1000400))
			case 2:
				n.Say(script.NpcString(1000401))
			case 3:
				n.Say(script.NpcString(1000402))
			}
			hateList := n.HateList()
			hateList.Refresh()
			hateList.Add(flagged, 2000)
		},
	}).Named("PartyPrivateWizard")
}

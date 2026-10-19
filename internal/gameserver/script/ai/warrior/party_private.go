package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyPrivate is PartyPrivateWarrior: a Warrior that follows its master
// and fights for it. A hit on any member of its party makes it hate the
// attacker by its weight point, and it turns on the target its master
// shouts. Once its master is dead it fights on its own; with no live master
// it wanders, and leaves the world at a 20 s check that finds it neither
// attacking nor casting.
func PartyPrivate() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior/PartyPrivateWarrior", Behavior: true, NPCs: partyPrivateIDs(), Hooks: PartyPrivateHooks()}
}

// partyPrivateIDs are PartyPrivateWarrior's own NPCs.
func partyPrivateIDs() []int32 {
	return []int32{
		20377, 20399, 20445, 20454, 20524, 20739, 20766, 27037, 29070, 29071, 29072, 29073, 29074, 29075,
		29076, 22079, 22095, 27189,
	}
}

// privateShouts are the lines a private says as it turns on the target its
// master shouted.
func privateShouts() []int32 { return []int32{1000292, 1000400, 1000401, 1000402} }

// deadMaster reports whether n has a master and that master is dead.
func deadMaster(n *script.NPC) bool {
	master := n.Master()
	return master != nil && master.Dead()
}

// PartyPrivateHooks are PartyPrivateWarrior's hooks, which its children
// derive theirs from.
func PartyPrivateHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "1005"):
				master := n.Master()
				if master != nil && !master.Dead() && n.Intention() != script.IntentionAttack && !n.InMyTerritory() {
					n.AbortAll()
					n.InstantTeleportTo(n.Master())
					n.RemoveAllAttackDesire()
				}
			case strings.EqualFold(e.Name, "1006"):
				master := n.Master()
				if master == nil || master.Dead() {
					if i := n.Intention(); i != script.IntentionAttack && i != script.IntentionCast {
						s.Spawner().DecayMe(n)
						return ""
					}
				}
			}
			return base.Timer(s, e)
		},
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			n := e.NPC
			if master := n.Master(); master != nil && !master.Dead() {
				n.AddFollowDesire(master, 50)
			} else {
				n.AddWanderDesire(5, 5)
			}
		},
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("1005", e.NPC, nil, 120000*time.Millisecond, 120000*time.Millisecond)
			s.StartTimerAtFixedRate("1006", e.NPC, nil, 20000*time.Millisecond, 20000*time.Millisecond)
			base.Created(s, e)
		},
		OnPartyAttacked: func(_ *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			if !called.IsMaster() && target.Playable() {
				hateRatio := damageHate(called, float64(e.Damage), defaultnpc.HateRatio(called, target))
				called.AddAttackDesire(target, float64(commons.TruncInt32Sat(hateRatio*float64(e.Damage)*float64(called.Int(script.SlotWeightPoint))*10)))
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				hateRatio := damageHate(n, float64(e.Damage), defaultnpc.HateRatio(n, attacker))
				if deadMaster(n) {
					n.AddAttackDesire(attacker, hateRatio*100)
				} else {
					n.AddAttackDesire(attacker, hateRatio*float64(e.Damage)*float64(n.Int(script.SlotWeightPoint))*10)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && deadMaster(called) {
				hateRatio := damageHate(called, float64(e.Damage), defaultnpc.HateRatio(called, attacker))
				called.AddAttackDesire(attacker, float64(commons.TruncInt32Sat(hateRatio*30)))
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if deadMaster(e.NPC) {
				base.SeeSpell(s, e)
			}
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if n.Dead() || e.EventID != 10002 || !n.HasMaster() {
				return
			}
			master, _ := s.Object(e.Arg1).(*script.NPC)
			if master == nil {
				return
			}
			flagged := s.Object(master.Int(script.SlotFlag))
			if flagged == nil {
				return
			}
			if script.Same(master, n.Master()) {
				if mostHated := n.MostHated(); mostHated != nil && script.Same(mostHated, flagged) {
					return
				}
				shouts := privateShouts()
				n.Say(script.NpcString(shouts[s.Rnd(len(shouts))]))
				n.RemoveAllAttackDesire()
				n.AddAttackDesire(flagged, 2000)
			}
		},
	}).Named("PartyPrivateWarrior")
}

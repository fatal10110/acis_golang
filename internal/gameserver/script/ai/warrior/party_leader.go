package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PartyLeader is PartyLeaderWarrior: a Warrior that leads its privates. It
// creates them when it spawns, or on a hit at its SummonPrivateRate chance,
// hates for every hit on itself, and brings a dead private back after its
// respawn delay. A player it hates most who falls under 40% of its HP gets
// shouted at and becomes its privates' target.
func PartyLeader() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior/PartyLeaderWarrior", Behavior: true, NPCs: partyLeaderIDs(), Hooks: PartyLeaderHooks()}
}

// partyLeaderIDs are PartyLeaderWarrior's own NPCs.
func partyLeaderIDs() []int32 {
	return []int32{27113, 27036, 27093, 27112, 27068, 27065, 27062, 27114, 27110, 27108}
}

// The lines a party leader says: as it calls its privates, and as it
// shouts a target to them.
func summonPrivatesShouts() []int32 { return []int32{1000294, 1000403, 1000404, 1000405} }

func targetShouts() []int32 { return []int32{1000291, 1000398, 1000399} }

// PartyLeaderHooks are PartyLeaderWarrior's hooks, which its children
// derive theirs from.
func PartyLeaderHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1007") {
				n := e.NPC
				if n.Intention() != script.IntentionAttack && !n.InMyTerritory() {
					n.InstantTeleportTo(n.SpawnLocation())
					n.RemoveAllAttackDesire()
				}
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if n.Int(script.SlotIntAI2) == 0 && int32(s.Rnd(100)) < n.IntParam("SummonPrivateRate") {
				defaultnpc.CreatePrivates(s, n)
				shouts := summonPrivatesShouts()
				n.Say(script.NpcString(shouts[s.Rnd(len(shouts))]))
				n.SetInt(script.SlotIntAI2, 1)
			}
			if _, isPlayer := attacker.(*script.Player); isPlayer && n.IntParam("ShoutTarget") == 1 && s.Rnd(100) < 50 && attacker.HPRatio() < 0.4 {
				if script.Same(n.MostHated(), attacker) {
					shouts := targetShouts()
					n.Say(script.NpcString(shouts[s.Rnd(len(shouts))], attacker.Name()))
					n.RemoveAllAttackDesire()
					n.AddAttackDesire(attacker, 1000)
					n.SetInt(script.SlotFlag, attacker.ObjectID())
					s.BroadcastScriptEvent(n, 10002, n.ObjectID(), 300)
				}
			}
			base.Attacked(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if n.IntParam("SummonPrivateRate") == 0 {
				defaultnpc.CreatePrivates(s, n)
				n.SetInt(script.SlotIntAI2, 1)
			} else {
				n.SetInt(script.SlotIntAI2, 0)
			}
			n.SetInt(script.SlotWeightPoint, 10)
			s.StartTimerAtFixedRate("1007", n, nil, 120000*time.Millisecond, 120000*time.Millisecond)
			base.Created(s, e)
		},
		OnPartyAttacked: func(_ *script.Script, e script.PartyAttacked) {
			called, target := e.Called, e.Target
			if called.IsMaster() && target.Playable() {
				hateRatio := damageHate(called, float64(e.Damage), defaultnpc.HateRatio(called, target))
				called.AddAttackDesire(target, float64(commons.TruncInt32Sat(hateRatio*float64(e.Damage)*float64(e.Caller.Int(script.SlotWeightPoint))*10)))
			}
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			if delay := s.Spawner().RespawnDelay(caller); delay != 0 {
				if called.IsMaster() && !called.Dead() {
					s.Spawner().ScheduleRespawn(caller, time.Duration(delay)*1000*time.Millisecond)
				}
			}
		},
	}).Named("PartyLeaderWarrior")
}

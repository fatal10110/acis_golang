// Package monsterbehavior holds MonsterBehavior, the two behaviors that
// derive from it, WarriorBehavior and WizardBehavior, and their children:
// monsters steered by their AI parameters (MovingAttack, Party_Type,
// AttackRange, ...). The first three bind no NPC of their own; their
// children do.
package monsterbehavior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
)

// MonsterBehavior is the MonsterBehavior behavior. It binds no NPC of its
// own.
func MonsterBehavior() script.Script {
	return script.Script{Dir: "ai/individual/MonsterBehavior", Behavior: true, Hooks: MonsterHooks()}
}

// followsBoss reports whether n is a private set to follow its master:
// Party_Type 1 with PrivateFollowBoss on, which it is unless set to 0.
func followsBoss(n *script.NPC) bool {
	return n.IntParam("Party_Type") == 1 && n.IntParamOr("PrivateFollowBoss", 1) == 1
}

// liveMaster returns n's master when it has one and that master is alive,
// else nil.
func liveMaster(n *script.NPC) *script.NPC {
	if m := n.Master(); m != nil && !m.Dead() {
		return m
	}
	return nil
}

// MonsterHooks are MonsterBehavior's hooks, which its children derive
// theirs from:
//   - with nothing to do, a monster that moves to attack (MovingAttack 1,
//     the default) follows its live master as a following private, and
//     wanders otherwise;
//   - a party leader (Party_Type 2) calls its privates as it spawns, and
//     brings a dead private back after its spawn's respawn delay while it
//     lives;
//   - a following private leaves the world, at a 20 s check after it spawns
//     or after its master dies, when it has no live master and neither
//     fights nor casts; every 120 s it also jumps back to a live master it
//     strayed from out of combat;
//   - a short-ranged monster (AttackRange 1) hit from closer than 100 runs
//     from that attacker 10 s later;
//   - a hit monster that is neither stunned nor paralyzed may recharge its
//     shots, by its SoulShot and SpiritShot rates.
func MonsterHooks() script.Hooks {
	base := monster.Hooks()
	return base.With(script.Hooks{
		OnNoDesire: func(s *script.Script, e script.NoDesire) {
			n := e.NPC
			if n.IntParamOr("MovingAttack", 1) == 1 {
				if master := liveMaster(n); n.IntParam("Party_Type") == 1 && master != nil {
					n.AddFollowDesire(master, 5)
				} else {
					n.AddWanderDesire(5, 5)
				}
			}
			base.NoDesire(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if n.IntParam("Party_Type") == 2 {
				defaultnpc.CreatePrivates(s, n)
			} else if followsBoss(n) && liveMaster(n) != nil {
				s.StartTimer("1005", n, nil, 120000*time.Millisecond)
				s.StartTimer("1006", n, nil, 20000*time.Millisecond)
			}
			if n.IntParam("AttackRange") == 1 {
				n.SetSlot(script.SlotCreature4, nil)
			}
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if n.IntParam("AttackRange") == 1 && n.Distance2D(attacker) < 100 && n.Int(script.SlotIntAI4) == 0 {
				n.SetInt(script.SlotIntAI4, 1)
				n.SetSlot(script.SlotCreature4, attacker)
				s.StartTimer("5001", n, nil, 10000*time.Millisecond)
			}
			if !n.Stunned() && !n.Paralyzed() {
				if n.IntParam("SoulShot") != 0 && int32(s.Rnd(100)) < n.IntParam("SoulShotRate") {
					n.RechargeShots(true, false)
				}
				if n.IntParam("SpiritShot") != 0 && int32(s.Rnd(100)) < n.IntParam("SpiritShotRate") {
					n.RechargeShots(false, true)
				}
			}
			base.Attacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "1005"):
				if followsBoss(n) {
					if liveMaster(n) == nil && !n.InCombat() && !n.CastingNow() {
						n.DeleteMe()
						return ""
					}
					if master := liveMaster(n); !n.InMyTerritory() && master != nil && !n.InCombat() {
						n.TeleportTo(master)
						n.RemoveAllAttackDesire()
					}
				}
				s.StartTimer("1005", n, e.Player, 120000*time.Millisecond)
			case strings.EqualFold(e.Name, "1006"):
				if followsBoss(n) && liveMaster(n) == nil && !n.InCombat() && !n.CastingNow() {
					n.DeleteMe()
					return ""
				}
			}
			if strings.EqualFold(e.Name, "5001") {
				if fled := n.Slot(script.SlotCreature4); n.Int(script.SlotIntAI4) == 1 && fled != nil {
					n.AddFleeDesire(fled, n.MaxDriftRange(), 10000000)
					n.SetInt(script.SlotIntAI4, 0)
					n.SetSlot(script.SlotCreature4, nil)
				}
			}
			return base.Timer(s, e)
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			switch partyType := called.IntParam("Party_Type"); {
			case partyType == 1:
				if called.IntParamOr("PrivateFollowBoss", 1) == 1 && called.HasMaster() && script.Same(caller, called.Master()) {
					s.StartTimer("1006", called, nil, 20000*time.Millisecond)
				}
			case partyType == 2 && called.IsMaster() && !called.Dead():
				s.Spawner().ScheduleRespawn(caller, time.Duration(s.Spawner().RespawnDelay(caller))*1000*time.Millisecond)
			}
			base.PartyDied(s, e)
		},
	}).Named("MonsterBehavior")
}

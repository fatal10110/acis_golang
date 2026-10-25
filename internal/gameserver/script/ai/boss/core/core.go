package core

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// cubeID is the teleport cube Core leaves twice when it dies.
const cubeID = 31842

// slotGreeted is 1 once Core has spoken its opening lines.
const slotGreeted = script.SlotIntAI0

// Core is the grand boss Core. It appears with its music and its
// privates, and brings each dying private back after 280 to 319 seconds
// while it lives. A hit on it, or on a creature of its clan, makes it speak
// its two opening lines the first time and, one time in 100, a taunt after
// that; a hit, or a spell it sees, from a player more than 8 levels above it
// makes it want a skill on that player. Dying, it plays its death music,
// speaks three last lines and leaves two teleport cubes for 15 minutes.
func Core() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: "ai/boss/core", Behavior: true, NPCs: []int32{29006}, Hooks: base.With(script.Hooks{
		// Core watches no door (it names no ChangedDoors), so this hook
		// never runs: the door's makers stay as their spawn times leave them.
		OnDoorChange: func(s *script.Script, e script.DoorChange) {
			if e.Door.Opened() {
				s.Spawner().StartSpawnTime("door_open", "[kuruma_parent]")
			} else {
				s.Spawner().StopSpawnTime("door_open", "[kuruma_parent]")
			}
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.PlayMusic("BS01_A")
			n.SetInt(slotGreeted, 0)

			defaultnpc.CreatePrivates(s, n)
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller, called := e.Caller, e.Called
			if !script.Same(caller, called) && called.IsMaster() && !called.Dead() {
				s.Spawner().ScheduleRespawn(caller, time.Duration(280+s.Rnd(40))*1000*time.Millisecond)
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Level() > n.Level()+8 {
				n.AddCastDesire(attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)
			}

			if n.Int(slotGreeted) == 0 {
				n.Say(script.NpcString(1000001))
				n.Say(script.NpcString(1000002))

				n.SetInt(slotGreeted, 1)
			} else if s.Rnd(100) < 1 {
				n.Say(script.NpcString(1000003))
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called := e.Called
			if !called.Dead() {
				if called.Int(slotGreeted) == 0 {
					called.Say(script.NpcString(1000001))
					called.Say(script.NpcString(1000002))
					called.SetInt(slotGreeted, 1)
				} else if s.Rnd(100) < 1 {
					called.Say(script.NpcString(1000003))
				}
			}
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			n.PlayMusic("BS02_D")

			n.Say(script.NpcString(1000004))
			n.Say(script.NpcString(1000005))
			n.Say(script.NpcString(1000006))

			s.Spawner().AddSpawn(cubeID, script.Loc{X: 16502, Y: 110165, Z: -6394}, false, 900000*time.Millisecond)
			s.Spawner().AddSpawn(cubeID, script.Loc{X: 18948, Y: 110166, Z: -6397}, false, 900000*time.Millisecond)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			if caster.Level() > n.Level()+8 {
				n.AddCastDesire(caster, skill.Ref{ID: 4215, Level: 1}, 1000000)
				return
			}

			base.SeeSpell(s, e)
		},
	})}
}

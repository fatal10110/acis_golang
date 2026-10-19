package maker

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// InstantSpawnRandom is the maker whose group's last NPC leaving wakes one
// of up to three npcmakers at random: a draw below its maker_cnt memo
// picks maker_name1, maker_name2 or maker_name3 for draws 0, 1 and 2, which
// runs script event 1001 with that NPC's respawn delay in seconds. An NPC
// leaving respawns nothing on its own.
func InstantSpawnRandom() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			g := e.Group
			if g.Alive() != 0 {
				return
			}
			switch m.Rnd(memoInt(g, "maker_cnt")) {
			case 0:
				sendNamed(g, "maker_name1", "1001", respawnSeconds(e.Spawn), 0)
			case 1:
				sendNamed(g, "maker_name2", "1001", respawnSeconds(e.Spawn), 0)
			case 2:
				sendNamed(g, "maker_name3", "1001", respawnSeconds(e.Spawn), 0)
			}
		},
	})}
}

// RandomSpawn is the maker that places one of its spawns, picked at
// random: at start, when the npcmaker starts at boot, the picked spawn
// places its whole total. Each NPC leaving picks a spawn again and, while
// that spawn is short of its total and the group under its maximum, brings
// one of its NPCs in after that spawn's respawn delay.
func RandomSpawn() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			if !e.Group.Maker().OnStart() {
				return
			}
			rndMs := randomSpawn(m, e.Group)
			for range rndMs.Total() {
				rndMs.Spawn()
			}
		},
		OnNPCDeleted: respawnRandomSpawn,
	})}
}

// RandomSpawnPawn is RandomSpawn's maker but for its start: when the
// npcmaker starts at boot, every spawn places its NPCs up to its total
// while the group is under its maximum.
func RandomSpawnPawn() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(_ *script.Maker, e script.MakerStart) {
			g := e.Group
			if !g.Maker().OnStart() {
				return
			}
			for _, ms := range g.Spawns() {
				if ms.Total() == ms.Spawned() {
					continue
				}
				for range ms.Total() - ms.Spawned() {
					if g.Maker().MaximumNPCs-g.Alive() > 0 {
						ms.Spawn()
					}
				}
			}
		},
		OnNPCDeleted: respawnRandomSpawn,
	})}
}

// respawnRandomSpawn is the npc deleted hook of the random spawn makers:
// a spawn picked at random, short of its total while the group is under
// its maximum, brings one NPC in after its respawn delay, an NPC out of
// the world back before a new one.
func respawnRandomSpawn(m *script.Maker, e script.MakerNPC) {
	g := e.Group
	rndMs := randomSpawn(m, g)
	if rndMs.Total()-rndMs.Spawned() <= 0 || g.Maker().MaximumNPCs-g.Alive() <= 0 {
		return
	}
	g.After(spawn.CalculateRespawnDelay(rndMs.Entry()), func() { respawnOrSpawn(rndMs) })
}

// randomSpawn draws one of g's spawns. A group with no spawn panics: the
// hook stops there.
func randomSpawn(m *script.Maker, g spawn.Group) spawn.GroupSpawn {
	spawns := g.Spawns()
	return spawns[m.Rnd(len(spawns))]
}

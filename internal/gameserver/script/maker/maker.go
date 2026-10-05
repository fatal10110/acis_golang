// Package maker contains the spawn makers: what an npcmaker does when it
// starts, when one of its NPCs enters or leaves the world, and when it
// receives a maker script event.
package maker

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Catalog maps each maker type with a maker of its own to its
// constructor. Every other type, default_maker included, runs Default.
func Catalog() script.MakerCatalog {
	return script.MakerCatalog{
		"no_on_start_maker": NoOnStart,
		"event_maker":       Event,
	}
}

// Default is the maker of every npcmaker whose type has none of its own:
// it spawns the group at start unless its spawn condition holds, keeps the
// NPCs out while it holds and respawns them otherwise.
func Default() script.Maker {
	return script.Maker{MakerHooks: defaultHooks()}
}

func defaultHooks() script.MakerHooks {
	return script.MakerHooks{
		OnStart: func(_ *script.Maker, e script.MakerStart) {
			if e.Group.Maker().OnStart() || !e.Group.Held() {
				spawnMissing(e.Group)
			}
		},
		OnNPCDBInfo: func(_ *script.Maker, e script.MakerSpawn) {
			if e.Spawn.Total()-e.Spawn.Spawned() > 0 {
				e.Spawn.Spawn()
			}
		},
		OnNPCCreated: func(_ *script.Maker, e script.MakerNPC) {
			if e.Group.Held() {
				e.NPC.Delete()
			}
		},
		OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
			if e.Group.Held() {
				return
			}
			respawnUnlessPermanent(e)
		},
		OnScriptEvent: defaultScriptEvent,
	}
}

// defaultScriptEvent answers the two default maker script events: 1000
// deletes every NPC of the group; 1001, unless the spawn condition holds,
// brings back up to the maker's maximum after int1 seconds, its NPCs out
// of the world first.
func defaultScriptEvent(_ *script.Maker, e script.MakerEvent) {
	g := e.Group
	switch {
	case strings.EqualFold(e.Name, "1000"):
		g.DeleteAll()
	case strings.EqualFold(e.Name, "1001"):
		if g.Held() {
			return
		}
		delay := time.Duration(e.Int1) * time.Second
		totalUnspawned := g.Maker().MaximumNPCs - g.Alive()
		if totalUnspawned <= 0 {
			return
		}
		for _, ms := range g.Spawns() {
			toSpawn := min(totalUnspawned, ms.Total()-ms.Spawned())
			for _, npc := range ms.NPCs() {
				if npc.Decayed() {
					npc.ScheduleRespawn(delay)
					toSpawn--
					totalUnspawned--
				}
			}
			for range max(toSpawn, 0) {
				g.After(delay, func() {
					if ms.Decayed() > 0 {
						ms.Respawn()
					} else {
						ms.Spawn()
					}
				})
				totalUnspawned--
			}
		}
	}
}

// NoOnStart is the default maker with nothing to do at start: its NPCs
// spawn only when something else starts them.
func NoOnStart() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(*script.Maker, script.MakerStart) {},
	})}
}

// Event is the maker whose NPCs live only while the SpawnEvents list names
// its EventName memo.
func Event() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(_ *script.Maker, e script.MakerStart) {
			if eventListed(e.Group) {
				spawnMissing(e.Group)
			}
		},
		OnNPCDBInfo: func(*script.Maker, script.MakerSpawn) {},
		OnNPCCreated: func(_ *script.Maker, e script.MakerNPC) {
			if !eventListed(e.Group) {
				e.NPC.Delete()
			}
		},
		OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
			if !eventListed(e.Group) {
				return
			}
			respawnUnlessPermanent(e)
		},
	})}
}

// eventListed reports whether the SpawnEvents list names g's EventName
// memo.
func eventListed(g spawn.Group) bool {
	return g.Listed(g.Maker().AIParams["EventName"])
}

// spawnMissing fills every spawn of g up to its total: a database-tracked
// spawn through the maker's npc DB info hook, any other one NPC at a time
// while the group is under its maximum.
func spawnMissing(g spawn.Group) {
	for _, ms := range g.Spawns() {
		if ms.Total() == ms.Spawned() {
			continue
		}
		if ms.Persisted() {
			ms.LoadDBInfo()
			continue
		}
		for range ms.Total() - ms.Spawned() {
			if g.Maker().MaximumNPCs-g.Alive() > 0 {
				ms.Spawn()
			}
		}
	}
}

// respawnUnlessPermanent schedules the NPC's respawn after its spawn's
// randomized delay, unless the spawn has none.
func respawnUnlessPermanent(e script.MakerNPC) {
	entry := e.Spawn.Entry()
	if entry.RespawnDelay != 0 {
		e.NPC.ScheduleRespawn(spawn.CalculateRespawnDelay(entry))
	}
}

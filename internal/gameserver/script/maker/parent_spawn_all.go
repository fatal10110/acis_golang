package maker

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// parentSpawnAllDefault is what a child maker key the memo does not
// declare reads; it names no npcmaker.
const parentSpawnAllDefault = "parent_spawn_all_default"

// ParentSpawnAll is DefaultUseDB's maker for a database-tracked unique NPC
// (the alias its unique_npc memo names) that brings its child npcmakers
// with it. At start every spawn short of its total runs the npc DB info
// hook, which places the spawn's NPC when its row is not saved dead and
// the whole spawn fits under the maximum, then runs script event 1001 on
// the A and B makers. The unique NPC entering runs 1001 on the A and B
// makers; leaving, it respawns after its spawn's delay while the group is
// under its maximum and runs 1000 on the A makers. Any other NPC leaving
// respawns nothing.
func ParentSpawnAll() script.Maker {
	// The memo keys naming the child npcmakers: the A makers follow the
	// unique NPC in and out, the B makers only in.
	makersA := []string{"maker_nameA1", "maker_nameA2", "maker_nameA3", "maker_nameA4"}
	makersB := []string{"maker_nameB1", "maker_nameB2", "maker_nameB3", "maker_nameB4"}
	base := DefaultUseDB().MakerHooks
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(_ *script.Maker, e script.MakerStart) {
			for _, ms := range e.Group.Spawns() {
				if ms.Total() != ms.Spawned() {
					ms.LoadDBInfo()
				}
			}
		},
		OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
			g := e.Group
			if !uniqueSpawn(g, e.Spawn) {
				return
			}
			respawnUnderMaximum(e)
			sendChildren(g, makersA, "1000")
		},
		OnNPCCreated: func(_ *script.Maker, e script.MakerNPC) {
			g := e.Group
			if !uniqueSpawn(g, e.Spawn) {
				return
			}
			sendChildren(g, makersA, "1001")
			sendChildren(g, makersB, "1001")
		},
		OnNPCDBInfo: func(_ *script.Maker, e script.MakerSpawn) {
			g, ms := e.Group, e.Spawn
			if !ms.Persisted() || ms.CheckDead() || g.Alive()+ms.Total() > g.Maker().MaximumNPCs {
				return
			}
			ms.Spawn()
			sendChildren(g, makersA, "1001")
			sendChildren(g, makersB, "1001")
		},
	})}
}

// sendChildren runs script event name on each npcmaker the memo keys name,
// in key order, skipping a key that names parentSpawnAllDefault.
func sendChildren(g spawn.Group, keys []string, name string) {
	for _, key := range keys {
		target := memoStringOr(g, key, parentSpawnAllDefault)
		if !strings.EqualFold(target, parentSpawnAllDefault) {
			g.SendMakerEvent(target, name, 0, 0)
		}
	}
}

package maker

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Farm is the maker of a contested hall's farm: at start its first spawn
// places one NPC and every later spawn runs the npc DB info hook, which
// brings back only an NPC whose database row is saved alive. The hall's
// siege events bring the later spawns in and out: onSiegeStart fills all
// but the last, onFlagWarFinalEvent fills them all, onSiegeEnd deletes
// them. Its NPCs never respawn on their own.
func Farm() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(_ *script.Maker, e script.MakerStart) {
			spawns := e.Group.Spawns()
			if len(spawns) > 1 {
				spawns[0].Spawn()
				for i := 1; i < len(spawns); i++ {
					spawns[i].LoadDBInfo()
				}
			}
		},
		OnNPCDeleted: func(*script.Maker, script.MakerNPC) {},
		OnNPCDBInfo: func(_ *script.Maker, e script.MakerSpawn) {
			ms := e.Spawn
			if ms.Total()-ms.Spawned() <= 0 || !ms.Persisted() || ms.CheckDead() {
				return
			}
			// A territory with no point to draw leaves the zero position: the
			// row then comes alive there instead of failing the hook.
			at, _ := ms.DrawSpawnLocation()
			if ms.CheckAlive(at) {
				ms.Spawn()
			}
		},
		OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
			spawns := e.Group.Spawns()
			switch {
			case strings.EqualFold(e.Name, "onSiegeStart"):
				for i := 1; i < len(spawns)-1; i++ {
					fillFarmSpawn(spawns[i])
				}
			case strings.EqualFold(e.Name, "onFlagWarFinalEvent"):
				for i := 1; i < len(spawns); i++ {
					fillFarmSpawn(spawns[i])
				}
			case strings.EqualFold(e.Name, "onSiegeEnd"):
				for i := 1; i < len(spawns); i++ {
					spawns[i].DeleteNPCs()
				}
			}
		},
	})}
}

// fillFarmSpawn brings ms up to its total, an NPC out of the world back
// before a new one.
func fillFarmSpawn(ms spawn.GroupSpawn) {
	for range ms.Total() - ms.Spawned() {
		if ms.Decayed() > 0 {
			ms.Respawn()
		} else {
			ms.Spawn()
		}
	}
}

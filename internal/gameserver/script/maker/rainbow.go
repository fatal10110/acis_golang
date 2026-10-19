package maker

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Rainbow is the Rainbow Springs hall maker: at start it places one NPC
// of its first spawn; maker script event onSiegeEnd places one NPC of each
// other spawn. It answers no other script event, and its NPCs never
// respawn on their own.
func Rainbow() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(_ *script.Maker, e script.MakerStart) {
			if def0 := firstSpawn(e.Group); def0 != nil {
				def0.Spawn()
			}
		},
		OnNPCDeleted: func(*script.Maker, script.MakerNPC) {},
		OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
			if !strings.EqualFold(e.Name, "onSiegeEnd") {
				return
			}
			spawns := e.Group.Spawns()
			for i0 := 1; i0 < len(spawns); i0++ {
				spawns[i0].Spawn()
			}
		},
	})}
}

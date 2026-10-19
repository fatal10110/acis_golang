package maker

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// WarriorPassiveWeakness is the open-door maker of the Imperial Tomb's
// first hall. Half a second after its group's last NPC leaves, timer 1000
// closes the doors its TDoorName1 to TDoorName4 memos name (the first two
// grave_pathway_1 and grave_pathway_2 when not declared) and opens the
// ones its WallDoorNameB1 to WallDoorNameB10 memos name.
func WarriorPassiveWeakness() script.Maker {
	base := openDoorHooks(&enabledMemo{})
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnTimer: func(m *script.Maker, e script.MakerTimer) {
			if !strings.EqualFold(e.Name, "1000") {
				return
			}
			g := e.Group
			namedDoor(m, memoStringOr(g, "TDoorName1", "grave_pathway_1")).Close()
			namedDoor(m, memoStringOr(g, "TDoorName2", "grave_pathway_2")).Close()
			for _, key := range []string{"TDoorName3", "TDoorName4"} {
				if name, ok := g.Maker().AIParams[key]; ok {
					namedDoor(m, name).Close()
				}
			}
			for _, key := range []string{
				"WallDoorNameB1", "WallDoorNameB2", "WallDoorNameB3", "WallDoorNameB4", "WallDoorNameB5",
				"WallDoorNameB6", "WallDoorNameB7", "WallDoorNameB8", "WallDoorNameB9", "WallDoorNameB10",
			} {
				if name, ok := g.Maker().AIParams[key]; ok {
					namedDoor(m, name).Open()
				}
			}
		},
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			g := e.Group
			if g.Alive() == 0 {
				g.After(500*time.Millisecond, func() { m.Timer(m, script.MakerTimer{Group: g, Name: "1000"}) })
			}
		},
	})}
}

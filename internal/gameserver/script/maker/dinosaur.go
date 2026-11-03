package maker

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Rhamphorhynchus is the maker of the rhamphorhynchi of Sailren's lair,
// the second of the lair's three dinosaur makers: Velociraptor's maker
// whose group's last NPC leaving, while the flag is down, runs 1001 on
// rune20_mb2017_03m1. Start sets the flag (memo i_ai0) to 0 and spawns
// nothing.
func Rhamphorhynchus() script.Maker {
	flag := &intMemo{key: "i_ai0"}
	base := velociraptorHooks(flag)
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(*script.Maker, script.MakerStart) {
			flag.set(0)
		},
		OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
			if e.Group.Alive() == 0 && flag.get() == 0 {
				e.Group.SendMakerEvent("rune20_mb2017_03m1", "1001", 0, 0)
			}
		},
	})}
}

// Tyrannosaurus is the maker of the tyrannosaurus of Sailren's lair, the
// last of the lair's three dinosaur makers: Velociraptor's maker whose
// group's last NPC leaving arms timer 1002 three minutes out, flag up or
// down. Timer 1002 runs 11042 on rune20_mb2017_04m1; every timer then runs
// Velociraptor's. Start sets the flag (memo i_ai0) to 0 and spawns nothing.
func Tyrannosaurus() script.Maker {
	flag := &intMemo{key: "i_ai0"}
	base := velociraptorHooks(flag)
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(*script.Maker, script.MakerStart) {
			flag.set(0)
		},
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			g := e.Group
			if g.Alive() == 0 {
				g.After(180000*time.Millisecond, func() { m.Timer(m, script.MakerTimer{Group: g, Name: "1002"}) })
			}
		},
		OnTimer: func(m *script.Maker, e script.MakerTimer) {
			if strings.EqualFold(e.Name, "1002") {
				e.Group.SendMakerEvent("rune20_mb2017_04m1", "11042", 0, 0)
			}
			base.Timer(m, e)
		},
	})}
}

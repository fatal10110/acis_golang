package maker

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// UndeadBandmaster is the open-door maker of the Imperial Tomb's band
// masters. Three seconds after start, and three seconds after each NPC
// the door opening places, timer 1000 sends the NPCs of its first, third,
// fifth and seventh spawns script event 10032 with the spawn's index. Its
// group's last NPC leaving opens the doors its TDoorName3 and TDoorName4
// memos name and runs maker script event 1000, int2 1, on the seven band
// npcmakers of the hall.
func UndeadBandmaster() script.Maker {
	enabled := &enabledMemo{}
	base := openDoorHooks(enabled)
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			armBandmasterTimer(m, e.Group)
			base.Start(m, e)
		},
		OnDoorEvent: func(m *script.Maker, e script.MakerDoor) {
			g := e.Group
			if e.Door.Opened() {
				if enabled.get() {
					return
				}
				enabled.set(true)
				for _, ms := range g.Spawns() {
					for range ms.Total() - ms.Spawned() {
						if g.Maker().MaximumNPCs-g.Alive() > 0 {
							ms.Spawn()
							armBandmasterTimer(m, g)
						}
					}
				}
				return
			}
			if !enabled.get() {
				return
			}
			enabled.set(false)
			g.DeleteAll()
		},
		OnTimer: func(_ *script.Maker, e script.MakerTimer) {
			if !strings.EqualFold(e.Name, "1000") {
				return
			}
			spawns := e.Group.Spawns()
			for i := 0; i <= 6; i += 2 {
				spawns[i].SendScriptEvent(10032, int32(i), 0)
			}
		},
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			switch e.Group.Alive() {
			case 6, 4, 2:
				// The hall's damage zones stay as they are.
			case 0:
				bandmasterLost(m, e.Group)
			}
		},
	})}
}

// bandmasterLost runs once the group's last NPC has left: it opens the
// doors the TDoorName3 and TDoorName4 memos name, when declared, and runs
// maker script event 1000, int2 1, on the hall's seven band npcmakers in
// order.
func bandmasterLost(m *script.Maker, g spawn.Group) {
	if name := memoStringOr(g, "TDoorName3", "TDoorName3_default"); !strings.EqualFold(name, "TDoorName3_default") {
		namedDoor(m, name).Open()
	}
	if name := memoStringOr(g, "TDoorName4", "TDoorName4_default"); !strings.EqualFold(name, "TDoorName4_default") {
		namedDoor(m, name).Open()
	}
	for _, maker := range []string{
		"godard32_2515_14m1",
		"godard32_2515_11m1",
		"godard32_2515_10m1",
		"godard32_2515_12m1",
		"godard32_2515_13m1",
		"godard32_2515_15m1",
		"godard32_2515_18m1",
	} {
		g.SendMakerEvent(maker, "1000", 0, 1)
	}
}

// armBandmasterTimer runs the maker's timer 1000 for g three seconds from
// now.
func armBandmasterTimer(m *script.Maker, g spawn.Group) {
	g.After(3000*time.Millisecond, func() { m.Timer(m, script.MakerTimer{Group: g, Name: "1000"}) })
}

// memoStringOr is the value the npcmaker's memo declares under key, def
// when it declares none.
func memoStringOr(g spawn.Group, key, def string) string {
	if v, ok := g.Maker().AIParams[key]; ok {
		return v
	}
	return def
}

// namedDoor is the door named name, ignoring case. No such door panics:
// the hook stops there.
func namedDoor(m *script.Maker, name string) *script.Door {
	d := m.Door(name)
	if d == nil {
		panic(fmt.Sprintf("maker: no door %q", name))
	}
	return d
}

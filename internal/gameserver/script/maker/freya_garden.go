package maker

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The makers of Freya's garden pass maker script events on to their NPCs.
// None answers the default maker's script events.

// FreyaDeaconKeeper is the maker of Freya's Steward: maker script event
// 10025 sends its first spawn's NPCs 10026, 10005 sends them 10005 and
// 11037 sends them 11037, each with 0 and 0.
func FreyaDeaconKeeper() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
			def0 := firstSpawn(e.Group)
			switch {
			case strings.EqualFold(e.Name, "10025"):
				if def0 != nil {
					def0.SendScriptEvent(10026, 0, 0)
				}
			case strings.EqualFold(e.Name, "10005"):
				if def0 != nil {
					def0.SendScriptEvent(10005, 0, 0)
				}
			case strings.EqualFold(e.Name, "11037"):
				if def0 != nil {
					def0.SendScriptEvent(11037, 0, 0)
				}
			}
		},
	})}
}

// SculptureGarden is the maker of the Garden Sculpture: maker script event
// 10005 sends its first spawn's NPCs 11038 with 0 and 0.
func SculptureGarden() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
			if !strings.EqualFold(e.Name, "10005") {
				return
			}
			if def0 := firstSpawn(e.Group); def0 != nil {
				def0.SendScriptEvent(11038, 0, 0)
			}
		},
	})}
}

// SculptureIceFairy is the maker of the Ice Fairy Sculpture, keeping a
// count (memo i_ai0) that start sets to 0 before the default maker's
// start. Maker script event 10005 adds one to the count and 10025 sets it
// back to 0; each then sends the first spawn's NPCs 10001 with the count.
func SculptureIceFairy() script.Maker {
	count := &intMemo{key: "i_ai0"}
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			count.set(0)
			base.Start(m, e)
		},
		OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
			def0 := firstSpawn(e.Group)
			switch {
			case strings.EqualFold(e.Name, "10005"):
				count.set(count.get() + 1)
				if def0 != nil {
					def0.SendScriptEvent(10001, int32(count.get()), 0)
				}
			case strings.EqualFold(e.Name, "10025"):
				count.set(0)
				if def0 != nil {
					def0.SendScriptEvent(10001, int32(count.get()), 0)
				}
			}
		},
	})}
}

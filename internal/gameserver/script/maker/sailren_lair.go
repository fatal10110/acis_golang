package maker

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// SailrenDummy is the maker of Sailren's lair dummy: maker script events
// 11046, 11047 and 11048, matched exactly, send its first spawn's NPCs the
// event of that number with 1 and 0. Every script event then runs the
// default maker's.
func SailrenDummy() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnScriptEvent: func(m *script.Maker, e script.MakerEvent) {
			switch e.Name {
			case "11046", "11047", "11048":
				passOnByNumber(e)
			}
			base.ScriptEvent(m, e)
		},
	})}
}

// StatueOfShilen is the maker of Shilen's Stone Statue: maker script
// events 11041, 11043, 11045, 11047 and 11050, matched exactly, send its
// first spawn's NPCs the event of that number with 1 and 0. Every script
// event then runs the default maker's.
func StatueOfShilen() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnScriptEvent: func(m *script.Maker, e script.MakerEvent) {
			switch e.Name {
			case "11041", "11043", "11045", "11047", "11050":
				passOnByNumber(e)
			}
			base.ScriptEvent(m, e)
		},
	})}
}

// passOnByNumber sends the first spawn's NPCs the script event whose
// number the maker script event e names, with 1 and 0.
func passOnByNumber(e script.MakerEvent) {
	def0 := firstSpawn(e.Group)
	if def0 == nil {
		return
	}
	// Only the makers' number cases reach here.
	id, _ := parseInt32(e.Name)
	def0.SendScriptEvent(id, 1, 0)
}

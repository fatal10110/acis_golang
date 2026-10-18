package maker

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// InstantSpawn is the maker whose group comes back as a whole: an NPC
// leaving the world respawns nothing on its own, but the group's last one
// runs the maker's own script event 1001 with no delay.
func InstantSpawn() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			if e.Group.Alive() == 0 {
				m.ScriptEvent(m, script.MakerEvent{Group: e.Group, Name: "1001"})
			}
		},
	})}
}

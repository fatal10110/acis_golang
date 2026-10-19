package maker

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// DefaultUseDBForFrintezza is the maker of Frintezza and his followers:
// DefaultUseDB's maker whose npc DB info hook places one NPC of the spawn
// whether or not the spawn is short of its total.
func DefaultUseDBForFrintezza() script.Maker {
	base := DefaultUseDB().MakerHooks
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnNPCDBInfo: func(_ *script.Maker, e script.MakerSpawn) {
			e.Spawn.Spawn()
		},
	})}
}

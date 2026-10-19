package maker

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// HallKeeperDefault is the maker of the Imperial Tomb hall keepers and the
// hall timer bomb: the default maker with nothing to do at start, its NPCs
// spawning only when a maker script event brings them.
func HallKeeperDefault() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(*script.Maker, script.MakerStart) {},
	})}
}

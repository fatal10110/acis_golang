package manager

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/rs/zerolog"
)

// testMakers is the maker registry of the makers ported with the engine.
func testMakers() MakerBehaviors {
	makers := script.MakerCatalog{"no_on_start_maker": maker.NoOnStart, "event_maker": maker.Event}
	return script.NewMakers(makers, maker.Default, script.MakerWorld{Night: func() bool { return false }}, zerolog.Nop())
}

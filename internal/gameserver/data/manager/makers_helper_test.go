package manager

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/rs/zerolog"
)

// testMakers is the production maker registry.
func testMakers() MakerBehaviors {
	return script.NewMakers(maker.Catalog(), maker.Default, zerolog.Nop())
}

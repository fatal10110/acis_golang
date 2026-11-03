package ai

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/catalog"
)

// scenarioMakers is every ported maker script of the catalog, which a
// scenario that starts its makers runs.
func scenarioMakers() script.MakerCatalog {
	out := script.MakerCatalog{}
	for _, file := range catalog.Makers() {
		for typ, ctor := range file {
			if ctor != nil {
				out[typ] = ctor
			}
		}
	}
	return out
}

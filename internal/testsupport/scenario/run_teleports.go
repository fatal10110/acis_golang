package scenario

import (
	"sync"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// shippedTeleportLists is the shipped gatekeeper teleport lists, loaded once
// per test binary.
var shippedTeleportLists struct {
	once     sync.Once
	teleport travel.TeleportTable
	instant  travel.InstantTable
	err      error
}

// teleportOptions are the boot options a teleports line asks for: the shipped
// teleport lists, charged for, with the half-price weekend hours read from
// the scenario's clock start, which is no weekend evening.
func (r *run) teleportOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	teleports := datapack.Path(t, "data", "xml", "teleports.xml")
	instants := datapack.Path(t, "data", "xml", "instantTeleports.xml")
	shippedTeleportLists.once.Do(func() {
		var err error
		if shippedTeleportLists.teleport, err = gamexml.LoadTeleports(teleports); err != nil {
			shippedTeleportLists.err = err
			return
		}
		shippedTeleportLists.instant, shippedTeleportLists.err = gamexml.LoadInstantTeleports(instants)
	})
	if shippedTeleportLists.err != nil {
		t.Fatalf("load shipped teleports: %v", shippedTeleportLists.err)
	}
	start := r.clockStart()
	return []gameservertest.Option{
		gameservertest.WithTeleports(shippedTeleportLists.teleport, shippedTeleportLists.instant, false, func() time.Time { return start }),
	}
}

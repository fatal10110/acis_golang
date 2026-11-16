package admin

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// heldBossZones is the boss zone store holding //reload zone open once it
// stored the permissions, until release is closed.
type heldBossZones struct {
	network.BossZoneStore
	saved   chan struct{}
	release chan struct{}
}

func (s *heldBossZones) SaveZones(ctx context.Context, zones []*zone.Boss) error {
	err := s.BossZoneStore.SaveZones(ctx, zones)
	close(s.saved)
	<-s.release
	return err
}

// TestAdminReloadZoneKeepsEntryGrantedMeanwhile: a lair entry granted, as a
// grand boss teleporter grants it through the zone the index resolves at
// the time, while //reload zone is still storing the permissions reaches
// the reloaded lair with its entry window: the player teleported in after
// the reload stays inside.
func TestAdminReloadZoneKeepsEntryGrantedMeanwhile(t *testing.T) {
	t.Parallel()
	oust := location.Location{X: 200, Y: 200, Z: 30}
	newLair := func() *zone.Boss {
		lair := zone.NewBoss(reloadZoneLair, reloadZoneCuboid(t, 5000, 6000, 5000, 6000))
		lair.InvadeWindow = 10 * time.Minute
		lair.OustLoc = oust
		return lair
	}
	before := zone.NewIndex()
	before.Add(newLair())
	after := zone.NewIndex()
	lair := newLair()
	after.Add(lair)
	store := &heldBossZones{saved: make(chan struct{}), release: make(chan struct{})}
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithZones(before),
		gameservertest.WithDataReloads(network.DataReloads{Zones: func() (*zone.Index, error) { return after, nil }}),
		gameservertest.WithBossZoneStore(func(db network.BossZoneStore) network.BossZoneStore {
			store.BossZoneStore = db
			return store
		}),
	)
	gm := srv.Client
	enterWorld(t, gm)
	visitor, visitorID := addPlayer(t, srv, "visitor", "Visitor", 0)

	gm.Send(encodeBuildCmd("reload zone"))
	<-store.saved
	k, ok := before.ByID(reloadZoneLair)
	if !ok {
		t.Fatal("the lair is not in the live zones")
	}
	k.(*zone.Boss).AllowEntry(visitorID, time.Minute)
	close(store.release)
	// The game master also sees the visitor arrive; the answer comes last.
	frames := exchangeZones(t, gm, nil)
	assertTexts(t, frames[len(frames)-1:], "Zones have been reloaded.")

	if got := lair.AllowedPlayers(); !slices.Equal(got, []int32{visitorID}) {
		t.Fatalf("reloaded lair allows %v, want the visitor %d", got, visitorID)
	}
	obj, ok := srv.State.Player(visitorID)
	if !ok {
		t.Fatal("visitor missing from world state")
	}
	character, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatal("visitor has no live character")
	}
	character.TeleportTo(5500, 5500, 30, 0)
	drain(t, visitor)
	appear(t, visitor)
	srv.Settle(t)
	if x, y, _ := srv.PlayerPosition(t, visitorID); x != 5500 || y != 5500 {
		t.Fatalf("visitor at (%d, %d) after entering the lair, want it still at (5500, 5500)", x, y)
	}
	if got := occupantIDs(lair); !slices.Equal(got, []int32{visitorID}) {
		t.Fatalf("reloaded lair holds %v, want the visitor", got)
	}
}

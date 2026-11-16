package npcs

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Reference: RequestPackageSend.runImpl (RequestPackageSend.java:56-63)
// needs only a Folk in interaction reach (or a GM) and leaves the target
// freight's active region as it was, so a package sent next to any civilian
// NPC lands untagged (visible at every keeper) or with the region of an
// earlier FreightChar. The server goes further on purpose: a package needs a
// warehouse keeper and carries that keeper's region.

// TestPackageSendNeedsKeeperAndTagsItsRegion sends a package beside a
// merchant, which moves nothing and charges nothing, then beside the
// warehouse keeper, which ships the sword tagged with the keeper's region
// without any FreightChar having picked a region first.
func TestPackageSendNeedsKeeperAndTagsItsRegion(t *testing.T) {
	t.Parallel()
	w := bootFreight(t, freightAdena, [][2]int32{{swordID, 1}})
	sword := w.items[swordID]
	kx, ky, _ := w.keeper.Position()
	here := world.RegionKey(kx, ky)
	if here == 0 {
		t.Fatal("keeper stands outside every region")
	}

	merchant := w.spawnFolk(t, folkTemplate("Merchant", 30001), -50)
	w.talkTo(t, merchant)
	w.c.Send(encodePackageSend(w.receiver.ID, whRow{sword, 1}))
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("package beside a merchant answered %x, want nothing", opcodes(frames))
	}
	if s, a := w.held(t, sword), w.adena(t); s != 1 || a != freightAdena {
		t.Fatalf("after the package beside a merchant: sword %d adena %d, want 1 and %d", s, a, freightAdena)
	}
	if got := rowsAt(saved(t, w.srv, w.receiver.ID), item.LocationFreight); len(got) != 0 {
		t.Fatalf("receiver's freight after the package beside a merchant = %v, want empty", got)
	}

	w.talkTo(t, w.keeper)
	w.c.Send(encodePackageSend(w.receiver.ID, whRow{sword, 1}))
	drainUntilQuiet(t, w.c)
	if s, a := w.held(t, sword), w.adena(t); s != 0 || a != freightAdena-freightPrice {
		t.Fatalf("after the package beside the keeper: sword %d adena %d, want 0 and %d", s, a, freightAdena-freightPrice)
	}
	w.srv.FlushItems(t)
	rows, err := w.srv.Items.ListByOwner(context.Background(), w.receiver.ID)
	if err != nil {
		t.Fatalf("list receiver items: %v", err)
	}
	var shipped *item.Instance
	for _, row := range rows {
		if row.ObjectID == sword {
			shipped = row
		}
	}
	if shipped == nil || shipped.Location != item.LocationFreight || shipped.LocationData != here {
		t.Fatalf("receiver's sword row = %+v, want in freight tagged %d", shipped, here)
	}
}

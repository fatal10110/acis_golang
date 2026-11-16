package npcs

import (
	"context"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: RequestPackageSend.runImpl (RequestPackageSend.java:56-63)
// needs only a Folk in interaction reach (or a GM) and leaves the target
// freight's active region as it was, so a package sent next to any civilian
// NPC lands untagged (visible at every keeper) or with the region of an
// earlier FreightChar. The server goes further on purpose: a package needs a
// warehouse keeper and carries that keeper's region.

// TestPackageSendNeedsKeeperAndTagsItsRegion sends a package beside a
// merchant, which moves nothing and charges nothing, then opens the
// receiver's freight at a keeper across a region edge, then sends beside
// the first warehouse keeper, which ships the sword tagged with its own
// region, not the one the earlier FreightChar picked.
func TestPackageSendNeedsKeeperAndTagsItsRegion(t *testing.T) {
	t.Parallel()
	w := bootFreightPrepared(t, func(srv *gameservertest.Server, sender int32, _ *player.Character) {
		moveToRegionEdge(t, srv, sender, 0)
	}, freightAdena, [][2]int32{{swordID, 1}})
	sword := w.items[swordID]
	kx, ky, _ := w.keeper.Position()
	here := world.RegionKey(kx, ky)
	if here == 0 {
		t.Fatal("keeper stands outside every region")
	}
	across := w.spawnFolk(t, folkTemplate("WarehouseKeeper", keeperID), -60)
	ax, ay, _ := across.Position()
	there := world.RegionKey(ax, ay)
	if there == here || there == 0 {
		t.Fatalf("second keeper's region %d, want a region other than %d", there, here)
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

	w.talkTo(t, across)
	w.openAnyNpcPage(t)
	frames := w.bypass(t, npcCommand(across, "FreightChar_"+strconv.Itoa(int(w.receiver.ID))))
	requireOpcodes(t, "FreightChar_ across the region edge", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseDepositList, serverpackets.OpcodeActionFailed)

	w.talkTo(t, w.keeper)
	w.c.Send(encodePackageSend(w.receiver.ID, whRow{sword, 1}))
	drainUntilQuiet(t, w.c)
	if s, a := w.held(t, sword), w.adena(t); s != 0 || a != freightAdena-freightPrice {
		t.Fatalf("after the package beside the keeper: sword %d adena %d, want 0 and %d", s, a, freightAdena-freightPrice)
	}
	if shipped := savedInstance(t, w.srv, w.receiver.ID, sword); shipped == nil || shipped.Location != item.LocationFreight || shipped.LocationData != here {
		t.Fatalf("receiver's sword row = %+v, want in freight tagged %d", shipped, here)
	}
}

// TestPackageSendByGMTagsTheGMsRegion sends a package as a GM with a
// merchant selected: no keeper is needed, the fee is charged and the sword
// ships tagged with the region the GM stands in, one region short of the
// keeper's.
func TestPackageSendByGMTagsTheGMsRegion(t *testing.T) {
	t.Parallel()
	levels, err := admin.NewData([]admin.AccessLevel{
		{Level: 0, AllowTransaction: true},
		{Level: freightGMLevel, IsGM: true, AllowTransaction: true},
	}, nil)
	if err != nil {
		t.Fatalf("admin.NewData: %v", err)
	}
	w := bootFreightPrepared(t, func(srv *gameservertest.Server, sender int32, _ *player.Character) {
		moveToRegionEdge(t, srv, sender, -1)
		setCharacterColumn(t, srv, sender, "accesslevel", freightGMLevel)
	}, freightAdena, [][2]int32{{swordID, 1}}, gameservertest.WithAdmin(levels))
	sword := w.items[swordID]
	gx, gy, _ := w.srv.PlayerPosition(t, w.player)
	kx, ky, _ := w.keeper.Position()
	gmRegion := world.RegionKey(gx, gy)
	if gmRegion == 0 || gmRegion == world.RegionKey(kx, ky) {
		t.Fatalf("GM's region %d, want a region other than the keeper's %d", gmRegion, world.RegionKey(kx, ky))
	}

	merchant := w.spawnFolk(t, folkTemplate("Merchant", 30001), -50)
	w.talkTo(t, merchant)
	w.c.Send(encodePackageSend(w.receiver.ID, whRow{sword, 1}))
	if frames := drainFrames(t, w.c); containsOpcode(frames, serverpackets.OpcodeSystemMessage) {
		t.Fatalf("GM's package answered %x, want no message", opcodes(frames))
	}
	if s, a := w.held(t, sword), w.adena(t); s != 0 || a != freightAdena-freightPrice {
		t.Fatalf("after the GM's package: sword %d adena %d, want 0 and %d", s, a, freightAdena-freightPrice)
	}
	if shipped := savedInstance(t, w.srv, w.receiver.ID, sword); shipped == nil || shipped.Location != item.LocationFreight || shipped.LocationData != gmRegion {
		t.Fatalf("receiver's sword row = %+v, want in freight tagged %d", shipped, gmRegion)
	}
}

// freightGMLevel is a GM access level that may trade.
const freightGMLevel = 7

// moveToRegionEdge moves the persisted character objID along x to the first
// x of its region, plus dx: the keeper enter spawns 50 to the east then
// stands in that region, and anything 60 to the west, or the character
// itself with dx -1, stands in the region before it.
func moveToRegionEdge(t *testing.T, srv *gameservertest.Server, objID int32, dx int) {
	t.Helper()
	var x, y int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT x, y FROM characters WHERE obj_Id = ?", objID).Scan(&x, &y); err != nil {
		t.Fatalf("read position of %d: %v", objID, err)
	}
	for world.RegionKey(x-1, y) == world.RegionKey(x, y) {
		x--
	}
	setCharacterColumn(t, srv, objID, "x", x+dx)
}

// savedInstance flushes item writes and returns ownerID's persisted row
// objectID, or nil.
func savedInstance(t *testing.T, srv *gameservertest.Server, ownerID, objectID int32) *item.Instance {
	t.Helper()
	srv.FlushItems(t)
	rows, err := srv.Items.ListByOwner(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list items of %d: %v", ownerID, err)
	}
	for _, row := range rows {
		if row.ObjectID == objectID {
			return row
		}
	}
	return nil
}

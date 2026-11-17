package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

// noStoreSummonZones places a no-store cuboid at X 2000..4000 and a
// no-summon-friend cuboid at X 6000..8000.
func noStoreSummonZones(t *testing.T) *zone.Index {
	t.Helper()
	noStoreForm, err := zone.NewCuboid(2_000, 4_000, -1_000, 1_000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	noSummonForm, err := zone.NewCuboid(6_000, 8_000, -1_000, 1_000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewNoStore(1, noStoreForm))
	zones.Add(zone.NewNoSummonFriend(2, noSummonForm))
	return zones
}

// TestNoStoreZoneRefusesPrivateStoreSetUp pins NoStoreZone.onEnter/onExit and
// Player.canOpenPrivateStore: inside a no-store zone the sell-store action is
// answered NO_PRIVATE_STORE_HERE and nothing else, the player stays out of
// store mode, and after leaving the zone the same action opens the manage
// window.
func TestNoStoreZoneRefusesPrivateStoreSetUp(t *testing.T) {
	t.Parallel()
	srv, character, objID := bootInZones(t, noStoreSummonZones(t))
	x, y, z := srv.PlayerPosition(t, objID)
	const actionPrivateStoreSell = 10

	teleportIntoWater(t, srv, character, 3_000, y, z)
	frames := framesBeforeItemList(t, srv.Client, encodeActionUse(actionPrivateStoreSell))
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("store set-up in the no-store zone answered opcodes %x, want only a SystemMessage", opcodesOfFrames(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageNoPrivateStoreHere)
	if got := character.OperateType(); got != privatestore.OperateNone {
		t.Fatalf("OperateType = %v after a refused set-up, want none", got)
	}

	character.TeleportTo(x, y, z, 0)
	readUntilQuiet(srv.Client)
	appear(t, srv.Client)
	readUntilQuiet(srv.Client)
	frames = framesBeforeItemList(t, srv.Client, encodeActionUse(actionPrivateStoreSell))
	if firstOpcode(frames, serverpackets.OpcodePrivateStoreManageListSell) < 0 {
		t.Fatalf("store set-up outside the zone answered opcodes %x, want the sell manage window", opcodesOfFrames(frames))
	}
	if got := character.OperateType(); got != privatestore.OperateSellManage {
		t.Fatalf("OperateType = %v after the set-up, want sell manage", got)
	}
}

// TestNoSummonFriendZoneFlagFollowsTheOccupant pins
// NoSummonFriendZone.onEnter/onExit: the flag the summon-friend and clan-gate
// gates read is set while the player stands in the zone and cleared on
// leaving it; the no-store zone does not set it.
func TestNoSummonFriendZoneFlagFollowsTheOccupant(t *testing.T) {
	t.Parallel()
	srv, character, objID := bootInZones(t, noStoreSummonZones(t))
	x, y, z := srv.PlayerPosition(t, objID)
	if character.NoSummonFriendZone() {
		t.Fatal("NoSummonFriendZone() = true at the spawn point, outside the zone")
	}

	teleportIntoWater(t, srv, character, 3_000, y, z)
	if character.NoSummonFriendZone() {
		t.Fatal("NoSummonFriendZone() = true inside the no-store zone")
	}
	teleportIntoWater(t, srv, character, 7_000, y, z)
	if !character.NoSummonFriendZone() {
		t.Fatal("NoSummonFriendZone() = false inside the no-summon-friend zone")
	}
	character.TeleportTo(x, y, z, 0)
	readUntilQuiet(srv.Client)
	appear(t, srv.Client)
	readUntilQuiet(srv.Client)
	if character.NoSummonFriendZone() {
		t.Fatal("NoSummonFriendZone() = true after leaving the zone")
	}
}

func opcodesOfFrames(frames [][]byte) []byte {
	ops := make([]byte, len(frames))
	for i, f := range frames {
		ops[i] = f[0]
	}
	return ops
}

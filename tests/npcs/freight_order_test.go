package npcs

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: PcFreight.getItems (PcFreight.java:57-65) collects the rows
// listed at the active region with Collectors.toSet(), a HashSet filled in
// the container's order (ItemContainer._items, a ConcurrentSkipListSet), and
// WarehouseWithdrawList (WarehouseWithdrawList.java:20-40) writes them in
// that set's iteration order. An item hashes as Objects.hash(objectId) =
// 31 + objectId (WorldObject.java:46-49); a HashMap of default capacity
// lists bucket by bucket, bucket = (h ^ h>>>16) & (n-1), n starting at 16
// and doubling once the size passes three quarters of it, and a bucket in
// insertion order.

// javaSetBucket is the bucket objectID's item falls in, in a HashSet of n
// buckets.
func javaSetBucket(objectID int32, n uint32) uint32 {
	h := uint32(31 + objectID)
	return (h ^ h>>16) & (n - 1)
}

// TestFreightWithdrawListOrder pins the freight withdrawal list's row
// order: twelve swords listed at this keeper come in the bucket order of a
// 16-bucket hash set filled newest first, and a sword tagged for another
// region neither shows nor counts towards the table's size.
func TestFreightWithdrawListOrder(t *testing.T) {
	t.Parallel()
	var listed []int32
	var hidden int32
	w := bootFreightPrepared(t, func(srv *gameservertest.Server, sender int32, receiver *player.Character) {
		// The account's characters start at one spot.
		x, y, _ := receiver.Position()
		seed := func(locationData int) int32 {
			id := srv.NewObjectID()
			if err := srv.Items.Create(context.Background(), sender, item.Instance{
				ObjectID: id, TemplateID: swordID, OwnerID: sender, Count: 1, Time: 1000,
				Location: item.LocationFreight, LocationData: locationData,
			}); err != nil {
				t.Fatalf("seed freight row: %v", err)
			}
			return id
		}
		for range 12 {
			listed = append(listed, seed(0))
		}
		hidden = seed(world.RegionKey(x+2*2048, y))
	}, whAdena, nil)

	frames := w.command(t, "WithdrawF")
	requireOpcodes(t, "WithdrawF", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed)
	_, _, order, _ := decodeWarehouseList(t, frames[1], serverpackets.OpcodeWarehouseWithdrawList)

	// Same entry time: the container lists the higher object id first.
	want := slices.Clone(listed)
	slices.SortFunc(want, func(a, b int32) int { return int(b) - int(a) })
	slices.SortStableFunc(want, func(a, b int32) int { return int(javaSetBucket(a, 16)) - int(javaSetBucket(b, 16)) })
	if !slices.Equal(order, want) {
		t.Fatalf("freight withdrawal list order = %v, want %v", order, want)
	}
	if slices.Contains(order, hidden) {
		t.Fatalf("freight withdrawal list %v shows %d, tagged for another region", order, hidden)
	}
}

package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

// handleRequestDropItem spends the drop reuse gate, then drops the item.
// The gate comes before anything else, so a drop refused further down still
// restarts it. A drop inside it is ignored without an answer, as the
// specified behavior does: the client keeps the dragged item in its
// inventory window and waits for nothing.
func (l *GameClientLink) handleRequestDropItem(_ context.Context, cs *connState, req clientpackets.RequestDropItem) error {
	if cs.live == nil {
		return nil
	}
	client, live := cs.client, cs.live
	// The gate belongs to this read loop, which stays parked in onLive
	// while this runs; it is timed on the player's queue clock.
	onLive(live, func() {
		if client.performFloodProtected(floodProtectorDropItem, l.playerConfig().DropItemDelay, live.Now()) {
			l.dropLiveItem(live, req)
		}
	})
	return nil
}

// unlessWarehouseDisabled runs handle unless AllowWarehouse is off. Off, the
// frame is neither read nor answered, as the specified behavior does: a
// malformed one is not even noticed, and the client, which closed its
// deposit window as it sent the list, waits for nothing.
func unlessWarehouseDisabled(handle inboundFunc) inboundFunc {
	return func(l *GameClientLink, ctx context.Context, cs *connState, payload []byte) error {
		if l.playerConfig().WarehouseDisabled {
			return nil
		}
		return handle(l, ctx, cs, payload)
	}
}

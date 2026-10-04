package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPetPickupCursedWeaponIsRefused: a pet ordered to loot a cursed
// weapon leaves it where it lies, and its owner reads FAILED_TO_PICKUP_S1
// naming the weapon. The reference (SummonAI.thinkPickUp) refuses the
// weapon with that message too, after taking it off the ground; the port
// refuses before the pet touches it, so the weapon is never lost.
func TestPetPickupCursedWeaponIsRefused(t *testing.T) {
	t.Parallel()
	const zariche int32 = 8190
	table, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{{ItemID: zariche}})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable: %v", err)
	}
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithCursedWeapons(table)})
	h.spawnWolf(t)
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	h.srv.SeedGroundItem(t, 0, zariche, 1, x, y, z)
	drainFrames(t, h.client)
	snaps := h.srv.GroundItems.Snapshots(nil)
	if len(snaps) != 1 {
		t.Fatalf("ground items = %d, want 1", len(snaps))
	}
	groundID := snaps[0].ObjectID

	h.client.Send(encodeRequestPetGetItem(groundID))
	assertFrameOpcode(t, mustRead(t, h.client, "refusal ActionFailed"), serverpackets.OpcodeActionFailed, "ActionFailed")
	frame := mustRead(t, h.client, "FAILED_TO_PICKUP_S1")
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if id, params, typ, itemID := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != serverpackets.SystemMessageFailedToPickupS1 ||
		params != 1 || typ != serverpackets.SystemMessageParamItemName || itemID != zariche {
		t.Fatalf("refusal = message %d params %d type %d item %d, want FAILED_TO_PICKUP_S1 naming %d", id, params, typ, itemID, zariche)
	}
	if frames := drainFrames(t, h.client); len(frames) != 0 {
		t.Fatalf("refusal sent %d more frames, want none", len(frames))
	}
	if _, ok := h.srv.State.Object(groundID); !ok {
		t.Fatal("cursed weapon left the ground")
	}
}

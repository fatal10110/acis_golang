package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestReturnCommandClearsSelectedPetAfterPetDelete pins the owner's frame
// order when the pet it has selected is dismissed: PetDelete first, then the
// selection clear — ActionFailed and the owner's own TargetUnselected — and
// the server-side selection is gone.
func TestReturnCommandClearsSelectedPetAfterPetDelete(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, h.client)
	owner := ownerTargetReader(t, h)
	if got := owner.Target(); got == nil || got.ObjectID() != pet.ObjectID() {
		t.Fatalf("Target() = %v after selecting the pet, want %d", got, pet.ObjectID())
	}

	h.client.Send(encodeRequestActionUse(19, false))
	for _, frame := range readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete") {
		if frame[0] == serverpackets.OpcodeTargetUnselected {
			t.Fatal("TargetUnselected arrived before PetDelete")
		}
	}
	assertFrameOpcode(t, mustRead(t, h.client, "target clear ActionFailed"), serverpackets.OpcodeActionFailed, "target clear ActionFailed")
	unselected := mustRead(t, h.client, "self TargetUnselected")
	assertFrameOpcode(t, unselected, serverpackets.OpcodeTargetUnselected, "self TargetUnselected")
	if id := wire.NewReader(unselected[1:]).ReadInt32(); id != h.ownerID {
		t.Fatalf("TargetUnselected object id = %d, want owner %d", id, h.ownerID)
	}
	if got := owner.Target(); got != nil {
		t.Fatalf("Target() = %d after the pet left, want none", got.ObjectID())
	}
	drainUntilQuiet(t, h.client)
}

func ownerTargetReader(t *testing.T, h *petWorld) interface{ Target() world.Tracked } {
	t.Helper()
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	owner, ok := obj.(interface{ Target() world.Tracked })
	if !ok {
		t.Fatalf("world player %T lacks Target", obj)
	}
	return owner
}

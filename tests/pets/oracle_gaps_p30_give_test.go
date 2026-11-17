package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// requireGiveRefusal gives the owner's adena to the pet and reads message as
// the only answer, with every unit still on the owner.
func (h *petWorld) requireGiveRefusal(t *testing.T, message int, adena int32) {
	t.Helper()
	frames := h.sendAndTick(t, encodeRequestGiveItemToPet(h.seededItem(t, item.AdenaID), adena))
	if len(frames) != 1 {
		t.Fatalf("refused give frames = %x, want only the refusal message", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[0], message)
	if got := h.ownerItemCount(t, item.AdenaID); got != int(adena) {
		t.Fatalf("owner adena rows = %d, want all %d kept", got, adena)
	}
	if got := h.petRowCount(t, item.AdenaID); got != 0 {
		t.Fatalf("pet adena rows = %d, want none", got)
	}
}

// TestGiveToPetRefusalsEndToEnd pins the refusal gates of RequestGiveItemToPet
// that depend on the pet itself: a dead pet takes nothing
// (CANNOT_GIVE_ITEMS_TO_DEAD_PET), a pet beyond the interaction distance is
// TARGET_TOO_FAR, and a pet with every slot taken cannot carry another kind
// of item. Each leaves the owner's stack whole and the pet's rows empty; the
// same give at 100 units goes through.
func TestGiveToPetRefusalsEndToEnd(t *testing.T) {
	t.Parallel()
	t.Run("dead pet", func(t *testing.T) {
		t.Parallel()
		h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 50})
		wolf, _ := h.spawnWolf(t)
		killPet(t, h, wolf)
		h.settleInventoryUpdates(t)
		h.requireGiveRefusal(t, serverpackets.SystemMessageCannotGiveItemsToDeadPet, 50)
	})

	t.Run("pet too far", func(t *testing.T) {
		t.Parallel()
		h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 50})
		wolf, _ := h.spawnWolf(t)
		x, y, z := h.srv.PlayerPosition(t, h.ownerID)
		runOnPetQueue(t, wolf, func() { wolf.SetXYZ(x+100, y, z) })
		h.settleInventoryUpdates(t)
		h.giveToPet(t, h.seededItem(t, item.AdenaID), 10)

		runOnPetQueue(t, wolf, func() { wolf.SetXYZ(x+500, y, z) })
		h.settleInventoryUpdates(t)
		frames := h.sendAndTick(t, encodeRequestGiveItemToPet(h.seededItem(t, item.AdenaID), 40))
		if len(frames) != 1 {
			t.Fatalf("far give frames = %x, want only TARGET_TOO_FAR", frameOpcodes(frames))
		}
		assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageTargetTooFar)
		h.requireAdena(t, wolf, 40, 10)
	})

	t.Run("pet slots full", func(t *testing.T) {
		t.Parallel()
		h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 50})
		h.spawnWolf(t)
		for range 12 {
			h.srv.GivePetItem(t, h.ownerID, 30, 1)
		}
		drainUntilQuiet(t, h.client)
		h.settleInventoryUpdates(t)
		h.requireGiveRefusal(t, serverpackets.SystemMessagePetCannotCarryMoreItems, 50)
	})
}

// TestGiveToPetFromTheDeadOwner pins that RequestGiveItemToPet has no
// life-state gate on the owner: a dead owner and an owner feigning death
// both hand their pet an item, told by the usual pet and owner inventory
// updates.
func TestGiveToPetFromTheDeadOwner(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		state func(t *testing.T, h *petWorld)
	}{
		{"dead", func(t *testing.T, h *petWorld) {
			obj, _ := h.srv.State.Player(h.ownerID)
			hostile := h.srv.SpawnHostileNPC(t)
			h.srv.Settle(t)
			owner := obj.(interface {
				SetSpawnProtection(bool)
				SetCP(float64)
				CurrentHP() int
				Dead() bool
				ReduceHP(float64, attackable.Combatant, modelskill.Definition)
			})
			owner.SetSpawnProtection(false)
			owner.SetCP(0)
			owner.ReduceHP(float64(owner.CurrentHP()), hostile, modelskill.Definition{})
			h.srv.Settle(t)
			if !owner.Dead() {
				t.Fatal("owner survived the monster's hit")
			}
		}},
		{"feigning death", func(t *testing.T, h *petWorld) {
			obj, _ := h.srv.State.Player(h.ownerID)
			c, ok := network.OnlineCharacter(obj)
			if !ok {
				t.Fatalf("%T is not an online character", obj)
			}
			var started bool
			runOn(t, h.srv.PlayerQueue(t, h.ownerID), func() { started = c.StartFakeDeath() })
			if !started {
				t.Fatal("StartFakeDeath reported no change")
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollar(t, seedItem{TemplateID: item.AdenaID, Count: 50})
			wolf, _ := h.spawnWolf(t)
			tt.state(t, h)
			drainUntilQuiet(t, h.client)
			h.settleInventoryUpdates(t)

			h.giveToPet(t, h.seededItem(t, item.AdenaID), 30)
			h.requireAdena(t, wolf, 20, 30)
		})
	}
}

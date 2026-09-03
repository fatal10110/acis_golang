package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestAutosaveSyncsCollarEnchantToPetLevel saves a live wolf whose collar
// still shows enchant 0: the periodic save writes pets.level and lifts the
// control item to that level, persisting the row and sending a modified
// InventoryUpdate.
func TestAutosaveSyncsCollarEnchantToPetLevel(t *testing.T) {
	h := bootOwnerWithCollar(t)
	actor, _ := h.spawnWolf(t)
	if got := h.liveCollarEnchant(t); got != 0 {
		t.Fatalf("collar enchant before save = %d, want 0", got)
	}
	if actor.Level() != wolfLevel {
		t.Fatalf("pet level = %d, want %d", actor.Level(), wolfLevel)
	}

	h.srv.TickAutosave()

	h.assertCollarEnchantedToPetLevel(t)
}

// TestReturnPetSyncsCollarEnchantToPetLevel covers the unsummon save: the
// same collar lift must land when the owner returns the pet, not only on
// the periodic autosave tick.
func TestReturnPetSyncsCollarEnchantToPetLevel(t *testing.T) {
	h := bootOwnerWithCollar(t)
	h.spawnWolf(t)

	h.returnPet(t)

	h.assertCollarEnchantedToPetLevel(t)
}

func (h *petWorld) assertCollarEnchantedToPetLevel(t *testing.T) {
	t.Helper()
	if got := h.liveCollarEnchant(t); got != wolfLevel {
		t.Fatalf("live collar enchant = %d, want pet level %d", got, wolfLevel)
	}
	entry := h.flushCollarInventoryUpdate(t)
	if entry.state != uint16(itemcontainer.UpdateModified) {
		t.Fatalf("InventoryUpdate state = %d, want modified (%d)", entry.state, itemcontainer.UpdateModified)
	}
	if int(entry.enchant) != wolfLevel {
		t.Fatalf("InventoryUpdate enchant = %d, want %d", entry.enchant, wolfLevel)
	}
	if got := h.persistedCollarEnchant(t); got != wolfLevel {
		t.Fatalf("persisted collar enchant = %d, want %d", got, wolfLevel)
	}
}

func (h *petWorld) liveCollarEnchant(t *testing.T) int {
	t.Helper()
	inst := h.ownerInventory(t).ItemByObjectID(h.collarID)
	if inst == nil {
		t.Fatalf("collar %d missing from owner inventory", h.collarID)
	}
	return inst.Snapshot().EnchantLevel
}

func (h *petWorld) persistedCollarEnchant(t *testing.T) int {
	t.Helper()
	h.srv.FlushItems(t)
	return mustPersistedItem(t, h.srv, h.ownerID, h.collarID).EnchantLevel
}

func (h *petWorld) ownerInventory(t *testing.T) *itemcontainer.Inventory {
	t.Helper()
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world")
	}
	holder, ok := obj.(interface {
		Inventory() *itemcontainer.Inventory
	})
	if !ok {
		t.Fatalf("owner %T has no Inventory", obj)
	}
	return holder.Inventory()
}

func (h *petWorld) flushCollarInventoryUpdate(t *testing.T) collarUpdate {
	t.Helper()
	h.srv.InventoryUpdates.Tick()
	frames := drainFrames(t, h.client)
	for _, frame := range frames {
		if len(frame) == 0 || frame[0] != serverpackets.OpcodeInventoryUpdate {
			continue
		}
		for _, e := range readCollarUpdates(t, frame) {
			if e.objID == h.collarID {
				return e
			}
		}
	}
	t.Fatalf("no InventoryUpdate for collar %d in opcodes %x", h.collarID, frameOpcodes(frames))
	return collarUpdate{}
}

type collarUpdate struct {
	state   uint16
	objID   int32
	enchant uint16
}

func readCollarUpdates(t *testing.T, frame []byte) []collarUpdate {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeInventoryUpdate, "InventoryUpdate")
	r := wire.NewReader(frame[1:])
	n := r.ReadUint16()
	entries := make([]collarUpdate, 0, n)
	for i := uint16(0); i < n; i++ {
		var e collarUpdate
		e.state = r.ReadUint16()
		r.ReadUint16()
		e.objID = r.ReadInt32()
		r.ReadInt32()
		r.ReadInt32()
		r.ReadUint16()
		r.ReadUint16()
		r.ReadUint16()
		r.ReadInt32()
		e.enchant = r.ReadUint16()
		r.ReadUint16()
		r.ReadInt32()
		r.ReadInt32()
		entries = append(entries, e)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read InventoryUpdate: %v", err)
	}
	return entries
}

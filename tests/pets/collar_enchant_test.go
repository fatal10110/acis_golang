package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestFirstSpawnSyncsCollarEnchantToPetLevel lifts a fresh collar (enchant 0)
// as soon as the pet's level is applied: live item, queued InventoryUpdate,
// persisted row, and PetInfo, without waiting for autosave or return.
func TestFirstSpawnSyncsCollarEnchantToPetLevel(t *testing.T) {
	h := bootOwnerWithCollar(t)
	actor, burst := h.spawnWolf(t)
	if actor.Level() != wolfLevel {
		t.Fatalf("pet level = %d, want %d", actor.Level(), wolfLevel)
	}
	if got := countOpcode(burst, serverpackets.OpcodePetInfo); got != 2 {
		t.Fatalf("spawn burst PetInfo count = %d, want 2 (collar-sync refresh + Discover); opcodes %x", got, frameOpcodes(burst))
	}
	h.assertCollarEnchantedTo(t, wolfLevel)
}

// TestLevelUpSyncsCollarEnchantToNewLevel drives a kill-reward level-up:
// the live setter lifts the collar to the new level, queues InventoryUpdate,
// and refreshes the owner with PetInfo at that moment.
func TestLevelUpSyncsCollarEnchantToNewLevel(t *testing.T) {
	h := bootOwnerWithCollar(t)
	actor, _ := h.spawnWolf(t)
	h.assertCollarEnchantedTo(t, wolfLevel)
	drainUntilQuiet(t, h.client)

	actor.AddExpAndSp(wolfNextLevelExp, 0)
	if actor.Level() != wolfNextLevel {
		t.Fatalf("pet level after exp = %d, want %d", actor.Level(), wolfNextLevel)
	}

	frames := drainFrames(t, h.client)
	if countOpcode(frames, serverpackets.OpcodePetInfo) == 0 {
		t.Fatalf("level-up frames missing PetInfo: opcodes %x", frameOpcodes(frames))
	}
	h.assertCollarEnchantedTo(t, wolfNextLevel)
}

// TestMatchingCollarEnchantSendsNoExtraPackets respawns a pet whose collar
// already shows the saved level: no InventoryUpdate and no extra PetInfo
// beyond the spawn announcement.
func TestMatchingCollarEnchantSendsNoExtraPackets(t *testing.T) {
	h := bootOwnerWithCollar(t)
	h.spawnWolf(t)
	h.returnPet(t)
	if _, ok := h.findCollarInventoryUpdate(t); !ok {
		t.Fatal("expected collar InventoryUpdate from the first level apply")
	}
	drainUntilQuiet(t, h.client)

	_, burst := h.spawnWolf(t)
	if got := h.liveCollarEnchant(t); got != wolfLevel {
		t.Fatalf("live collar enchant on respawn = %d, want %d", got, wolfLevel)
	}
	if countOpcode(burst, serverpackets.OpcodePetInfo) != 1 {
		t.Fatalf("matching spawn PetInfo count = %d, want 1 Discover announcement; opcodes %x",
			countOpcode(burst, serverpackets.OpcodePetInfo), frameOpcodes(burst))
	}
	if _, ok := h.findCollarInventoryUpdate(t); ok {
		t.Fatal("matching collar sent InventoryUpdate")
	}
}

// TestAutosaveSyncsCollarEnchantToPetLevel keeps the save-path lift: a
// periodic save still writes the pets row and leaves the control item at
// the saved level after the live setter has already applied it.
func TestAutosaveSyncsCollarEnchantToPetLevel(t *testing.T) {
	h := bootOwnerWithCollar(t)
	actor, _ := h.spawnWolf(t)
	if actor.Level() != wolfLevel {
		t.Fatalf("pet level = %d, want %d", actor.Level(), wolfLevel)
	}

	h.srv.TickAutosave()

	h.assertCollarEnchantedTo(t, wolfLevel)
}

// TestReturnPetSyncsCollarEnchantToPetLevel covers the unsummon save: the
// same collar lift must land when the owner returns the pet, not only on
// the periodic autosave tick.
func TestReturnPetSyncsCollarEnchantToPetLevel(t *testing.T) {
	h := bootOwnerWithCollar(t)
	h.spawnWolf(t)

	h.returnPet(t)

	h.assertCollarEnchantedTo(t, wolfLevel)
}

// TestLogoutSyncsCollarEnchantToPetLevel covers detachLivePlayer: the
// collar lift must run before the owner inventory flush, or logout writes
// the pre-lift enchant and never rewrites the row. Persistence is read
// from the items table as detach left it — FlushItems after despawn would
// not replay a lift that missed that flush.
func TestLogoutSyncsCollarEnchantToPetLevel(t *testing.T) {
	h := bootOwnerWithCollar(t)
	h.spawnWolf(t)

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	assertFrameOpcode(t, mustRead(t, h.client, "LeaveWorld"), serverpackets.OpcodeLeaveWorld, "LeaveWorld")
	if !h.client.AwaitClose(2 * time.Second) {
		t.Fatal("logout did not close the connection")
	}
	waitFor(t, "owner left world", func() bool {
		_, ok := h.srv.State.Player(h.ownerID)
		return !ok
	})

	if got := mustPersistedItem(t, h.srv, h.ownerID, h.collarID).EnchantLevel; got != wolfLevel {
		t.Fatalf("persisted collar enchant after logout = %d, want %d", got, wolfLevel)
	}
}

func (h *petWorld) assertCollarEnchantedTo(t *testing.T, level int) {
	t.Helper()
	if got := h.liveCollarEnchant(t); got != level {
		t.Fatalf("live collar enchant = %d, want pet level %d", got, level)
	}
	entry, ok := h.findCollarInventoryUpdate(t)
	if !ok {
		t.Fatalf("no InventoryUpdate for collar %d", h.collarID)
	}
	if entry.state != uint16(itemcontainer.UpdateModified) {
		t.Fatalf("InventoryUpdate state = %d, want modified (%d)", entry.state, itemcontainer.UpdateModified)
	}
	if int(entry.enchant) != level {
		t.Fatalf("InventoryUpdate enchant = %d, want %d", entry.enchant, level)
	}
	if got := h.persistedCollarEnchant(t); got != level {
		t.Fatalf("persisted collar enchant = %d, want %d", got, level)
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

func (h *petWorld) findCollarInventoryUpdate(t *testing.T) (collarUpdate, bool) {
	t.Helper()
	h.srv.InventoryUpdates.Tick()
	frames := drainFrames(t, h.client)
	for _, frame := range frames {
		if len(frame) == 0 || frame[0] != serverpackets.OpcodeInventoryUpdate {
			continue
		}
		for _, e := range readCollarUpdates(t, frame) {
			if e.objID == h.collarID {
				return e, true
			}
		}
	}
	return collarUpdate{}, false
}

func countOpcode(frames [][]byte, opcode byte) int {
	n := 0
	for _, frame := range frames {
		if len(frame) > 0 && frame[0] == opcode {
			n++
		}
	}
	return n
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

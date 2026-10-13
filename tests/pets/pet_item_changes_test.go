package pets

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// petUpdateEntry is one PetInventoryUpdate entry, without its category
// fields and body slot.
type petUpdateEntry struct {
	state       uint16
	objectID    int32
	templateID  int32
	count       int32
	customType1 uint16
	equipped    uint16
	enchant     uint16
	customType2 uint16
}

func readPetUpdateEntries(t *testing.T, frame []byte) []petUpdateEntry {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodePetInventoryUpdate, "PetInventoryUpdate")
	r := wire.NewReader(frame[1:])
	n := r.ReadUint16()
	entries := make([]petUpdateEntry, 0, n)
	for range n {
		var e petUpdateEntry
		e.state = r.ReadUint16()
		r.ReadUint16() // category
		e.objectID = r.ReadInt32()
		e.templateID = r.ReadInt32()
		e.count = r.ReadInt32()
		r.ReadUint16() // sub-category
		e.customType1 = r.ReadUint16()
		e.equipped = r.ReadUint16()
		r.ReadInt32() // body slot
		e.enchant = r.ReadUint16()
		e.customType2 = r.ReadUint16()
		entries = append(entries, e)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read PetInventoryUpdate: %v", err)
	}
	return entries
}

// TestTakeBackWornWeaponListsTakeOffBeforeRemoval takes an equipped wolf
// weapon back. Leaving the pet's inventory takes the weapon off first, so the
// pet's update lists the weapon modified (no longer worn) ahead of its
// removal, as the item log records the take-off ahead of the removal.
func TestTakeBackWornWeaponListsTakeOffBeforeRemoval(t *testing.T) {
	t.Parallel()
	h, _, weaponID := bootArmedWolf(t)
	h.sendAndTick(t, encodeRequestPetUseItem(weaponID))

	frames := h.sendAndTick(t, encodeRequestGetItemFromPet(weaponID, 1))
	var updates [][]byte
	for _, frame := range frames {
		if len(frame) > 0 && frame[0] == serverpackets.OpcodePetInventoryUpdate {
			updates = append(updates, frame)
		}
	}
	if len(updates) != 1 {
		t.Fatalf("take-back frames = opcodes %x, want one PetInventoryUpdate", frameOpcodes(frames))
	}
	want := []petUpdateEntry{
		{state: uint16(itemcontainer.UpdateModified), objectID: weaponID, templateID: fixturePetWeaponID, count: 1},
		{state: uint16(itemcontainer.UpdateRemoved), objectID: weaponID, templateID: fixturePetWeaponID, count: 1},
	}
	if got := readPetUpdateEntries(t, updates[0]); !slices.Equal(got, want) {
		t.Fatalf("take-back PetInventoryUpdate = %+v, want %+v", got, want)
	}
}

// collarUpdateEntry is one InventoryUpdate entry's state and custom types.
type collarUpdateEntry struct {
	state       uint16
	objectID    int32
	customType2 uint16
}

func readInventoryUpdateCustomTypes(t *testing.T, frame []byte) []collarUpdateEntry {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeInventoryUpdate, "InventoryUpdate")
	r := wire.NewReader(frame[1:])
	n := r.ReadUint16()
	entries := make([]collarUpdateEntry, 0, n)
	for range n {
		var e collarUpdateEntry
		e.state = r.ReadUint16()
		r.ReadUint16() // category
		e.objectID = r.ReadInt32()
		r.ReadInt32()  // template id
		r.ReadInt32()  // count
		r.ReadUint16() // sub-category
		r.ReadUint16() // custom type 1
		r.ReadUint16() // equipped
		r.ReadInt32()  // body slot
		r.ReadUint16() // enchant
		e.customType2 = r.ReadUint16()
		r.ReadInt32() // augmentation
		r.ReadInt32() // mana
		entries = append(entries, e)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read InventoryUpdate: %v", err)
	}
	return entries
}

// unstackedCollarTemplates is the shared item table with the wolf collar not
// stackable, as every shipped collar is, so its changes reach the item log.
func unstackedCollarTemplates() *item.Table {
	all := gameservertest.ItemTemplates().All()
	out := make([]*item.Template, 0, len(all))
	for _, tmpl := range all {
		if tmpl.ID == wolfCollarID {
			collar := *tmpl
			collar.Stackable = false
			tmpl = &collar
		}
		out = append(out, tmpl)
	}
	return item.NewTable(out)
}

// TestRenamePetListsCollarModified names an unnamed pet. The collar is marked
// named through the owner's inventory: the next InventoryUpdate lists the
// collar modified with custom type 2 set, and the item log records one
// modified change of the collar.
func TestRenamePetListsCollarModified(t *testing.T) {
	t.Parallel()
	var log petItemLog
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithItemLog(zerolog.New(&log)),
		gameservertest.WithItemTemplates(unstackedCollarTemplates()),
	})
	h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	log.take(t)

	renameTo(t, h, "Fenrir")
	h.srv.InventoryUpdates.Tick()
	frames := drainFrames(t, h.client)
	var got []collarUpdateEntry
	for _, frame := range frames {
		if len(frame) > 0 && frame[0] == serverpackets.OpcodeInventoryUpdate {
			got = append(got, readInventoryUpdateCustomTypes(t, frame)...)
		}
	}
	want := []collarUpdateEntry{{state: uint16(itemcontainer.UpdateModified), objectID: h.collarID, customType2: 1}}
	if !slices.Equal(got, want) {
		t.Fatalf("InventoryUpdate entries after the rename = %+v (frames %x), want %+v", got, frameOpcodes(frames), want)
	}

	wantLog := []petItemRecord{{"MODIFIED", "Owner", h.ownerID, h.collarID, "INVENTORY"}}
	if gotLog := log.take(t); !slices.Equal(gotLog, wantLog) {
		t.Fatalf("item log after the rename = %+v, want %+v", gotLog, wantLog)
	}
}

package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestPetWeightChangeRefreshesOwnerStatusAndInfo drives a give that changes
// the pet's carried weight: the batching tick follows the PetInventoryUpdate
// with PetStatusUpdate and then a PetInfo carrying the new weight, as the
// reference's PetInventory.updateWeight does, and another player watching
// the pet receives its NpcInfo. A weightless give leaves the weight
// unchanged and sends none of them.
func TestPetWeightChangeRefreshesOwnerStatusAndInfo(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t,
		seedItem{TemplateID: wolfFoodID, Count: 5},
		seedItem{TemplateID: item.AdenaID, Count: 100},
	)
	h.srv.SeedCharacterFor(t, "player2", "Watcher", 1, 0)
	observer := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, h.client)
	pet, _ := h.spawnWolf(t)
	drainUntilQuiet(t, observer)

	frames := h.giveToPet(t, h.seededItem(t, item.AdenaID), 10)
	if n := countOpcode(frames, serverpackets.OpcodePetStatusUpdate) + countOpcode(frames, serverpackets.OpcodePetInfo); n != 0 {
		t.Fatalf("weightless give sent a pet refresh: opcodes %x", frameOpcodes(frames))
	}
	if n := countNPCInfoFor(drainFrames(t, observer), pet.ObjectID()); n != 0 {
		t.Fatalf("weightless give sent the observer %d pet NpcInfo, want 0", n)
	}

	frames = h.giveToPet(t, h.seededItem(t, wolfFoodID), 3)
	var got []byte
	var info []byte
	for _, frame := range frames {
		switch frame[0] {
		case serverpackets.OpcodePetInventoryUpdate, serverpackets.OpcodePetStatusUpdate:
			got = append(got, frame[0])
		case serverpackets.OpcodePetInfo:
			got = append(got, frame[0])
			info = frame
		}
	}
	want := []byte{serverpackets.OpcodePetInventoryUpdate, serverpackets.OpcodePetStatusUpdate, serverpackets.OpcodePetInfo}
	if string(got) != string(want) {
		t.Fatalf("weight-change pet frames = %x, want %x (all opcodes %x)", got, want, frameOpcodes(frames))
	}
	if weight := readPetInfoWeight(t, info); weight != 120 {
		t.Fatalf("refreshed PetInfo current weight = %d, want 120 (3 food x 40)", weight)
	}
	if n := countNPCInfoFor(drainFrames(t, observer), pet.ObjectID()); n != 1 {
		t.Fatalf("weight change sent the observer %d pet NpcInfo, want 1", n)
	}
}

// countNPCInfoFor counts the NpcInfo frames describing objectID.
func countNPCInfoFor(frames [][]byte, objectID int32) int {
	n := 0
	for _, frame := range frames {
		if len(frame) > 0 && frame[0] == serverpackets.OpcodeNPCInfo && wire.NewReader(frame[1:]).ReadInt32() == objectID {
			n++
		}
	}
	return n
}

// readPetInfoWeight returns PetInfo's current carried-weight field.
func readPetInfoWeight(t *testing.T, frame []byte) int32 {
	t.Helper()
	readPetInfoName(t, frame)
	r := wire.NewReader(frame[1:])
	for i := 0; i < 19; i++ {
		r.ReadInt32()
	}
	for i := 0; i < 4; i++ {
		r.ReadFloat64()
	}
	for i := 0; i < 3; i++ {
		r.ReadInt32()
	}
	for i := 0; i < 5; i++ {
		r.ReadUint8()
	}
	r.ReadString()            // name
	r.ReadString()            // title
	for i := 0; i < 11; i++ { // 1, pvp flag, karma, fed/max, hp/max, mp/max, sp, level
		r.ReadInt32()
	}
	for i := 0; i < 3; i++ { // exp, this-level exp, next-level exp
		r.ReadInt64()
	}
	weight := r.ReadInt32()
	if err := r.Err(); err != nil {
		t.Fatalf("read PetInfo weight: %v", err)
	}
	return weight
}

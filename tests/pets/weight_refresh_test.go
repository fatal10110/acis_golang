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
// reference's PetInventory.updateWeight does. A weightless give leaves the
// weight unchanged and sends neither.
func TestPetWeightChangeRefreshesOwnerStatusAndInfo(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t,
		seedItem{TemplateID: wolfFoodID, Count: 5},
		seedItem{TemplateID: item.AdenaID, Count: 100},
	)
	h.spawnWolf(t)

	frames := h.giveToPet(t, h.seededItem(t, item.AdenaID), 10)
	if n := countOpcode(frames, serverpackets.OpcodePetStatusUpdate) + countOpcode(frames, serverpackets.OpcodePetInfo); n != 0 {
		t.Fatalf("weightless give sent a pet refresh: opcodes %x", frameOpcodes(frames))
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

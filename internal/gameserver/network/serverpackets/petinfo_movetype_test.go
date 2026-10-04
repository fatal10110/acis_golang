package serverpackets

import "testing"

// TestFramePetInfoWritesMoveType pins PetInfo's move type byte
// (PetInfo.java:105): 1 for a swimming summon, 0 on the ground.
func TestFramePetInfoWritesMoveType(t *testing.T) {
	const fromEnd = 1 + 2 + 1 + 4 + 4 // move type, then uint16, team, soulshots, spiritshots
	for _, moveType := range []int{0, 1} {
		got := framePayload(t, FramePetInfo(PetInfoSnapshot{Name: "Wolf", MoveType: moveType}))
		if b := got[len(got)-fromEnd]; int(b) != moveType {
			t.Errorf("FramePetInfo(MoveType %d) move type byte = %d", moveType, b)
		}
	}
}

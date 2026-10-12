package serverpackets

import (
	"encoding/binary"
	"testing"
)

// TestFrameServerObjectInfoHidesHPWhenNotAttackable pins the HP pair of
// the stationary view: a viewer the NPC is not attackable by sees 0 for
// both, whatever the NPC's HP.
func TestFrameServerObjectInfoHidesHPWhenNotAttackable(t *testing.T) {
	got := framePayload(t, FrameServerObjectInfo(NPCInfoSnapshot{
		ObjectID: 7, TemplateID: 123, Name: "G", CurrentHP: 70, MaxHP: 100,
	}))
	// Opcode, two ids, the name "G" and its terminator, then the flag.
	if flag := binary.LittleEndian.Uint32(got[1+8+4:]); flag != 0 {
		t.Fatalf("attackable = %d, want 0", flag)
	}
	tail := got[len(got)-16:]
	hp, maxHP := binary.LittleEndian.Uint32(tail), binary.LittleEndian.Uint32(tail[4:])
	if hp != 0 || maxHP != 0 {
		t.Fatalf("HP pair = %d/%d, want 0/0 for a viewer the NPC is not attackable by", hp, maxHP)
	}
}

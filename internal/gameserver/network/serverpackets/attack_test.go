package serverpackets

import (
	"bytes"
	"testing"
)

func TestFrameAttack(t *testing.T) {
	got := framePayload(t, FrameAttack(AttackSnapshot{
		AttackerID: 268476516,
		X:          -71440,
		Y:          258000,
		Z:          -3104,
		Hits: []AttackHit{
			{TargetID: 268480061, Damage: 37, Flags: AttackHitSoulshot | AttackHitCritical | AttackHitShield},
			{TargetID: 7, Damage: 0, Flags: AttackHitMiss},
		},
	}))
	want := []byte{
		0x05,
		0x64, 0xa0, 0x00, 0x10,
		0x3d, 0xae, 0x00, 0x10,
		0x25, 0x00, 0x00, 0x00,
		0x70,
		0xf0, 0xe8, 0xfe, 0xff,
		0xd0, 0xef, 0x03, 0x00,
		0xe0, 0xf3, 0xff, 0xff,
		0x01, 0x00,
		0x07, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x80,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAttack() = %x, want %x", got, want)
	}
}

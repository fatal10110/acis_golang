package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// The fishing packets' bytes, field by field as ExFishingStart,
// ExFishingEnd, ExFishingStartCombat and ExFishingHpRegen write them:
// 0xfe, the sub-opcode as a short, then the fields.
func TestFishingPacketBytes(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  []byte
		want []byte
	}{
		{
			"ExFishingStart",
			framePayload(t, FrameExFishingStart(0x10203040, -1, location.Location{X: 260, Y: -20, Z: 10}, true, false)),
			[]byte{
				0xfe, 0x13, 0x00,
				0x40, 0x30, 0x20, 0x10,
				0xff, 0xff, 0xff, 0xff, // fish type -1
				0x04, 0x01, 0x00, 0x00, // 260
				0xec, 0xff, 0xff, 0xff, // -20
				0x0a, 0x00, 0x00, 0x00, // 10
				0x01, // night lure
				0x00, // no ranking button
			},
		},
		{
			"ExFishingEnd",
			framePayload(t, FrameExFishingEnd(7, true)),
			[]byte{0xfe, 0x14, 0x00, 0x07, 0x00, 0x00, 0x00, 0x01},
		},
		{
			"ExFishingStartCombat",
			framePayload(t, FrameExFishingStartCombat(7, 24, 100, 1, 2, 1)),
			[]byte{
				0xfe, 0x15, 0x00,
				0x07, 0x00, 0x00, 0x00,
				0x18, 0x00, 0x00, 0x00, // 24 s
				0x64, 0x00, 0x00, 0x00, // 100 HP
				0x01, 0x02, 0x01, // mode, lure type, deceptive
			},
		},
		{
			"ExFishingHpRegen",
			framePayload(t, FrameExFishingHpRegen(7, FishingHPRegen{Time: 23, HP: 74, Mode: 1, GoodUse: 2, Anim: 1, Penalty: 50, Deceptive: 1})),
			[]byte{
				0xfe, 0x16, 0x00,
				0x07, 0x00, 0x00, 0x00,
				0x17, 0x00, 0x00, 0x00, // 23 s
				0x4a, 0x00, 0x00, 0x00, // 74 HP
				0x01, 0x02, 0x01, // mode, good use, anim
				0x32, 0x00, 0x00, 0x00, // penalty 50
				0x01, // deceptive
			},
		},
	} {
		if !bytes.Equal(tc.got, tc.want) {
			t.Errorf("%s = %x, want %x", tc.name, tc.got, tc.want)
		}
	}
}

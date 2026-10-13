package serverpackets

import (
	"bytes"
	"testing"
)

// The fixtures follow the Olympiad match packets' writeImpl in the
// reference (ExOlympiadMode, ExOlympiadMatchEnd, ExOlympiadUserInfo): 0xfe,
// the little-endian sub-opcode, then each field as written, a byte as one
// byte and a string as UTF-16LE closed by a zero character.
func TestOlympiadMatchPacketBytes(t *testing.T) {
	cases := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"ExOlympiadMode side 2", framePayload(t, FrameExOlympiadMode(OlympiadModeSide2)), []byte{0xfe, 0x2b, 0x00, 2}},
		{"ExOlympiadMode none", framePayload(t, FrameExOlympiadMode(OlympiadModeNone)), []byte{0xfe, 0x2b, 0x00, 0}},
		{"ExOlympiadMatchEnd", framePayload(t, FrameExOlympiadMatchEnd()), []byte{0xfe, 0x2c, 0x00}},
		{"ExOlympiadUserInfo", framePayload(t, FrameExOlympiadUserInfo(OlympiadUserInfo{
			Side: 1, ObjectID: 0x10203040, Name: "Ab", ClassID: 88,
			HP: 1, MaxHP: 0x0102, CP: 5, MaxCP: 6,
		})), []byte{
			0xfe, 0x29, 0x00,
			1,
			0x40, 0x30, 0x20, 0x10,
			'A', 0, 'b', 0, 0, 0,
			88, 0, 0, 0,
			1, 0, 0, 0,
			0x02, 0x01, 0, 0,
			5, 0, 0, 0,
			6, 0, 0, 0,
		}},
	}
	for _, tc := range cases {
		if !bytes.Equal(tc.got, tc.want) {
			t.Errorf("%s = % x, want % x", tc.name, tc.got, tc.want)
		}
	}
}

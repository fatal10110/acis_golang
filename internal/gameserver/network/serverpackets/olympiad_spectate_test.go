package serverpackets

import (
	"bytes"
	"testing"
)

// The fixtures follow ExOlympiadSpelledInfo's and ExOlympiadMode's
// writeImpl in the reference: 0xfe, the little-endian sub-opcode, then the
// competitor, the effect count and per effect its skill, its level as two
// bytes and its whole seconds left (writeEffect without the toggle), or the
// mode as one byte.
func TestOlympiadSpectatePacketBytes(t *testing.T) {
	cases := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"ExOlympiadMode observer", framePayload(t, FrameExOlympiadMode(OlympiadModeObserver)), []byte{0xfe, 0x2b, 0x00, 3}},
		{"ExOlympiadSpelledInfo none", framePayload(t, FrameExOlympiadSpelledInfo(0x10203040, nil)), []byte{
			0xfe, 0x2a, 0x00,
			0x40, 0x30, 0x20, 0x10,
			0, 0, 0, 0,
		}},
		{"ExOlympiadSpelledInfo two", framePayload(t, FrameExOlympiadSpelledInfo(7, []AbnormalStatusEffect{
			{SkillID: 1204, Level: 2, DurationMillis: 1_199_999},
			{SkillID: 0x0102, Level: 0x0304, DurationMillis: -1},
		})), []byte{
			0xfe, 0x2a, 0x00,
			7, 0, 0, 0,
			2, 0, 0, 0,
			0xb4, 0x04, 0, 0, 2, 0, 0xaf, 0x04, 0, 0,
			0x02, 0x01, 0, 0, 0x04, 0x03, 0, 0, 0, 0,
		}},
	}
	for _, tc := range cases {
		if !bytes.Equal(tc.got, tc.want) {
			t.Errorf("%s = % x, want % x", tc.name, tc.got, tc.want)
		}
	}
}

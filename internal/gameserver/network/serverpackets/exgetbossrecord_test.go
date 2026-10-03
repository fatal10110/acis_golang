package serverpackets

import (
	"bytes"
	"testing"
)

// TestFrameExGetBossRecord pins the layout against the reference's
// ExGetBossRecord bytes (tests/party/testdata/raid_points.golden, players
// 3 and 4): a record lists each boss with a zero after its points; no
// record is an empty list padded with three zeros.
func TestFrameExGetBossRecord(t *testing.T) {
	withRecord := []byte{
		0xfe, 0x33, 0x00,
		0x00, 0x00, 0x00, 0x00, // rank
		0x00, 0x00, 0x00, 0x00, // total
		0x01, 0x00, 0x00, 0x00, // bosses
		0xa9, 0x61, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	if got := framePayload(t, FrameExGetBossRecord(0, 0, []BossRecordEntry{{BossID: 25001}}, true)); !bytes.Equal(got, withRecord) {
		t.Fatalf("with a record = %x, want %x", got, withRecord)
	}
	none := []byte{0xfe, 0x33, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if got := framePayload(t, FrameExGetBossRecord(0, 0, nil, false)); !bytes.Equal(got, none) {
		t.Fatalf("without a record = %x, want %x", got, none)
	}
	ranked := []byte{
		0xfe, 0x33, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x2d, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x20, 0x62, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x62, 0x00, 0x00, 0x28, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	if got := framePayload(t, FrameExGetBossRecord(2, 45, []BossRecordEntry{{BossID: 25120, Points: 5}, {BossID: 25088, Points: 40}}, true)); !bytes.Equal(got, ranked) {
		t.Fatalf("ranked = %x, want %x", got, ranked)
	}
}

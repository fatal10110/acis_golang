package serverpackets

import (
	"bytes"
	"testing"
)

// Golden frame: a 2-byte little-endian length that counts itself, then the
// body the reference writes: 0xf7, the hall id (D), then one byte (C) per
// slot in the order HP recovery, MP recovery, statue (unused), experience
// recovery, teleport, crystal (unused), curtains, hangings (unused),
// support magic, flag (unused), front platform, item creation.
func TestClanHallDecorationGolden(t *testing.T) {
	got := frameBytes(t, FrameClanHallDecoration(ClanHallDecoration{
		HallID:       0x22,
		RestoreHP:    1,
		RestoreMP:    2,
		RestoreExp:   3,
		Teleport:     4,
		Curtains:     5,
		SupportMagic: 6,
		Fixtures:     7,
		CreateItem:   8,
	}))
	want := []byte{
		0x13, 0x00, 0xf7,
		0x22, 0x00, 0x00, 0x00,
		0x01, 0x02, 0x00, 0x03, 0x04, 0x00, 0x05, 0x00, 0x06, 0x00, 0x07, 0x08,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ClanHallDecoration frame = % x, want % x", got, want)
	}
}

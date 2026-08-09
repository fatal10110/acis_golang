package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFrameRelationChanged(t *testing.T) {
	got := framePayload(t, FrameRelationChanged(RelationChangedInfo{
		ObjectID:         12345,
		Relation:         RelationPvPFlag | RelationHasKarma,
		IsAutoAttackable: true,
		Karma:            500,
		PvPFlag:          1,
	}))

	want := []byte{OpcodeRelationChanged}
	for _, v := range []uint32{12345, RelationPvPFlag | RelationHasKarma, 1, 500, 1} {
		want = binary.LittleEndian.AppendUint32(want, v)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameRelationChanged() = %x, want %x", got, want)
	}
}

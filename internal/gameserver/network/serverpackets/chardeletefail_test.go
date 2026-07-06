package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeCharDeleteFail(t *testing.T) {
	tests := []struct {
		reason CharDeleteFailReason
		want   int32
	}{
		{CharDeleteFailReasonDeletionFailed, 1},
		{CharDeleteFailReasonClanMemberMayNotDelete, 2},
		{CharDeleteFailReasonClanLeaderMayNotDelete, 3},
	}
	for _, tt := range tests {
		got := EncodeCharDeleteFail(tt.reason)

		want := []byte{OpcodeCharDeleteFail}
		want = binary.LittleEndian.AppendUint32(want, uint32(tt.want))

		if !bytes.Equal(got, want) {
			t.Errorf("EncodeCharDeleteFail(%v) = %x, want %x", tt.reason, got, want)
		}
	}
}

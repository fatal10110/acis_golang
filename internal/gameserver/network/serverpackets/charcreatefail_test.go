package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFrameCharCreateFail(t *testing.T) {
	tests := []struct {
		reason CharCreateFailReason
		want   int32
	}{
		{CharCreateFailReasonCreationFailed, 0},
		{CharCreateFailReasonTooManyCharacters, 1},
		{CharCreateFailReasonNameAlreadyExists, 2},
		{CharCreateFailReason16EngChars, 3},
		{CharCreateFailReasonIncorrectName, 4},
		{CharCreateFailReasonCreateNotAllowed, 5},
		{CharCreateFailReasonChooseAnotherServer, 6},
	}
	for _, tt := range tests {
		got := framePayload(t, FrameCharCreateFail(tt.reason))

		want := []byte{OpcodeCharCreateFail}
		want = binary.LittleEndian.AppendUint32(want, uint32(tt.want))

		if !bytes.Equal(got, want) {
			t.Errorf("FrameCharCreateFail(%v) = %x, want %x", tt.reason, got, want)
		}
	}
}

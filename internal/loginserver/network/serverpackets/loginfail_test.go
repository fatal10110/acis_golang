package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeLoginFail(t *testing.T) {
	tests := []struct {
		name   string
		reason LoginFailReason
	}{
		{"system error", LoginFailSystemError},
		{"dual box", LoginFailDualBox},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeLoginFail(tt.reason)

			var want []byte
			want = append(want, OpcodeLoginFail)
			want = binary.LittleEndian.AppendUint32(want, uint32(tt.reason))

			if !bytes.Equal(got, want) {
				t.Errorf("EncodeLoginFail(%v) = %x, want %x", tt.reason, got, want)
			}
		})
	}
}

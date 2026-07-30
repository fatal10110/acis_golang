package network

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// TestExpSpGainMessage pins the amount-based message choice for one
// experience/SP gain against literal wire bytes, so a changed id or parameter
// order fails here instead of reaching a client.
func TestExpSpGainMessage(t *testing.T) {
	tests := []struct {
		name string
		exp  int64
		sp   int
		want []byte
	}{
		{
			name: "sp only",
			exp:  0,
			sp:   25,
			want: []byte{
				0x64,                   // SystemMessage
				0x4b, 0x01, 0x00, 0x00, // ACQUIRED_S1_SP (331)
				0x01, 0x00, 0x00, 0x00, // one param
				0x01, 0x00, 0x00, 0x00, // number param
				0x19, 0x00, 0x00, 0x00, // 25
			},
		},
		{
			name: "experience only",
			exp:  1000,
			sp:   0,
			want: []byte{
				0x64,                   // SystemMessage
				0x2d, 0x00, 0x00, 0x00, // EARNED_S1_EXPERIENCE (45)
				0x01, 0x00, 0x00, 0x00, // one param
				0x01, 0x00, 0x00, 0x00, // number param
				0xe8, 0x03, 0x00, 0x00, // 1000
			},
		},
		{
			name: "both",
			exp:  1000,
			sp:   25,
			want: []byte{
				0x64,                   // SystemMessage
				0x5f, 0x00, 0x00, 0x00, // YOU_EARNED_S1_EXP_AND_S2_SP (95)
				0x02, 0x00, 0x00, 0x00, // two params
				0x01, 0x00, 0x00, 0x00, // number param
				0xe8, 0x03, 0x00, 0x00, // 1000
				0x01, 0x00, 0x00, 0x00, // number param
				0x19, 0x00, 0x00, 0x00, // 25
			},
		},
		{
			name: "neither",
			exp:  0,
			sp:   0,
			want: []byte{
				0x64,                   // SystemMessage
				0x5f, 0x00, 0x00, 0x00, // YOU_EARNED_S1_EXP_AND_S2_SP (95)
				0x02, 0x00, 0x00, 0x00, // two params
				0x01, 0x00, 0x00, 0x00, // number param
				0x00, 0x00, 0x00, 0x00, // 0
				0x01, 0x00, 0x00, 0x00, // number param
				0x00, 0x00, 0x00, 0x00, // 0
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			frame := expSpGainMessage(tc.exp, tc.sp)
			defer frame.Release()
			got := frame.Bytes()[wire.FrameHeaderSize:]
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("expSpGainMessage(%d, %d) = %x, want %x", tc.exp, tc.sp, got, tc.want)
			}
		})
	}
}

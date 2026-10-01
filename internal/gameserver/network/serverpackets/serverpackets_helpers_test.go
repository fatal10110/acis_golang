package serverpackets

import (
	"encoding/binary"
	"math"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// ---- from charselectinfo_test.go ----
func appendF64(b []byte, v float64) []byte {
	return binary.LittleEndian.AppendUint64(b, math.Float64bits(v))
}

func encodeUTF16Z(s string) []byte {
	var out []byte
	for _, u := range utf16.Encode([]rune(s)) {
		out = binary.LittleEndian.AppendUint16(out, u)
	}
	return binary.LittleEndian.AppendUint16(out, 0)
}

// ---- from enterworld_packets_test.go ----
func appendD(b []byte, v int32) []byte {
	return binary.LittleEndian.AppendUint32(b, uint32(v))
}

func appendH(b []byte, v uint16) []byte {
	return binary.LittleEndian.AppendUint16(b, v)
}

// ---- from frame_test.go ----
func frameBytes(t *testing.T, frame wire.Frame) []byte {
	t.Helper()
	t.Cleanup(frame.Release)
	return frame.Bytes()
}

func framePayload(t *testing.T, frame wire.Frame) []byte {
	t.Helper()
	bytes := frameBytes(t, frame)
	if len(bytes) < 2 {
		t.Fatalf("frame length = %d, want header", len(bytes))
	}
	return bytes[2:]
}

// ---- from variation_test.go ----
func appendQ(b []byte, v int64) []byte {
	return binary.LittleEndian.AppendUint64(b, uint64(v))
}

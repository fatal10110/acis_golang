package serverpackets

import (
	"bytes"
	"testing"
)

// TestFrameServerDownCountdown pins the countdown notice a timed shutdown
// broadcasts: SystemMessage 1 with the seconds left as its one number.
func TestFrameServerDownCountdown(t *testing.T) {
	got := framePayload(t, FrameSystemMessageNumber(SystemMessageServerDownInS1Seconds, 65))
	want := []byte{
		0x64,                   // SystemMessage
		0x01, 0x00, 0x00, 0x00, // 1: the server will be coming down in S1 seconds
		0x01, 0x00, 0x00, 0x00, // one param
		0x01, 0x00, 0x00, 0x00, // number param
		0x41, 0x00, 0x00, 0x00, // 65 seconds
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("countdown frame = %x, want %x", got, want)
	}
}

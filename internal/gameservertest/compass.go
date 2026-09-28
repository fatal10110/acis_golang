package gameservertest

import (
	"bytes"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// ReadInitialCompass consumes the zone code sent before the EnterWorld burst.
// skip lists observer or status frames that may precede the player's own reply.
func ReadInitialCompass(tb testing.TB, c *testsupport.ScriptedClient, skip ...byte) []byte {
	tb.Helper()
	for {
		frame := c.Read()
		if slices.Contains(skip, frame[0]) {
			continue
		}
		if len(frame) != 7 || !bytes.Equal(frame[:3], []byte{0xfe, 0x32, 0}) || frame[3] < 0x0b || frame[3] > 0x0f || !bytes.Equal(frame[4:], []byte{0, 0, 0}) {
			tb.Fatalf("EnterWorld first own frame = %x, want ExSetCompassZoneCode", frame)
		}
		return frame
	}
}

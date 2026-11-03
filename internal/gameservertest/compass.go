package gameservertest

import (
	"bytes"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// ExSetCompassZoneCode's SIEGEWARZONE2 and PVPZONE codes.
const (
	compassSiegeWarZone = 0x0b
	compassPvPZone      = 0x0e
)

// enteredCombatZone is the SystemMessage frame (ENTERED_COMBAT_ZONE) an
// active siege zone or an arena sends on entry, just before the compass
// code: message id 283, no parameters.
var enteredCombatZone = []byte{serverpackets.OpcodeSystemMessage, 0x1b, 0x01, 0, 0, 0, 0, 0, 0}

// ReadInitialCompass consumes the zone code sent after EtcStatusUpdate.
// skip lists observer or status frames that may precede the zone code.
// A login inside an active siege zone or an arena hears
// ENTERED_COMBAT_ZONE first, once for each such zone, as the zone's entry
// runs before the compass update; that message is accepted only when the
// compass code that follows is the siege or PvP one.
func ReadInitialCompass(tb testing.TB, c *testsupport.ScriptedClient, skip ...byte) []byte {
	tb.Helper()
	combatNotice := false
	for {
		frame := c.Read()
		// One notice per combat zone entered: an arena inside an active
		// siege zone sends two.
		if bytes.Equal(frame, enteredCombatZone) {
			combatNotice = true
			continue
		}
		if !combatNotice && slices.Contains(skip, frame[0]) {
			continue
		}
		if len(frame) != 7 || !bytes.Equal(frame[:3], []byte{0xfe, 0x32, 0}) || frame[3] < 0x0b || frame[3] > 0x0f || !bytes.Equal(frame[4:], []byte{0, 0, 0}) {
			tb.Fatalf("EnterWorld compass frame = %x, want ExSetCompassZoneCode", frame)
		}
		if combatNotice && frame[3] != compassSiegeWarZone && frame[3] != compassPvPZone {
			tb.Fatalf("EnterWorld ENTERED_COMBAT_ZONE before compass code %#x, want only before the siege code %#x or the PvP code %#x", frame[3], compassSiegeWarZone, compassPvPZone)
		}
		return frame
	}
}

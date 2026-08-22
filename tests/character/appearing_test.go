package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAppearingAnswersUserInfoOutsideTeleportWindow pins issue #1634: a
// live client can send opcode 0x30 outside the restart-point teleport
// window (Teleporting() already false), and the server must keep answering
// UserInfo for that case (Appearing.java:17-24).
func TestAppearingAnswersUserInfoOutsideTeleportWindow(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected

	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	drainQuiet(t, c)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("opcode = %#x, want UserInfo (%#x)", reply[0], serverpackets.OpcodeUserInfo)
	}
}

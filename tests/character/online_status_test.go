package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// persistedOnline reads the characters row's online flag straight from the
// database, the surface external DB consumers watch.
func persistedOnline(t *testing.T, srv *gameservertest.Server, objID int32) int {
	t.Helper()
	var online int
	if err := srv.DB.QueryRow("SELECT online FROM characters WHERE obj_Id = ?", objID).Scan(&online); err != nil {
		t.Fatalf("read online flag: %v", err)
	}
	return online
}

// TestOnlineFlagTracksGamePresence requires the characters row to read
// online=1 while the character is in game and online=0 again after logout,
// so external DB consumers see presence the whole session long.
func TestOnlineFlagTracksGamePresence(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)

	if got := persistedOnline(t, srv, objID); got != 0 {
		t.Fatalf("online before enter world = %d, want 0", got)
	}

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)

	if got := persistedOnline(t, srv, objID); got != 1 {
		t.Fatalf("online after enter world = %d, want 1", got)
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeLeaveWorld {
		t.Fatalf("logout opcode = %#x, want LeaveWorld (%#x)", reply[0], serverpackets.OpcodeLeaveWorld)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("post-logout opcode = %#x, want ActionFailed from detach's unconditional cast-stop ack (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
	c.ExpectClosed()

	if got := persistedOnline(t, srv, objID); got != 0 {
		t.Fatalf("online after logout = %d, want 0", got)
	}
}

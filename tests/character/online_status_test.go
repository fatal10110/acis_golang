package character

import (
	"testing"
	"time"

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

// TestOnlineTimeAccruesAcrossSessions requires a restored playtime base to
// survive the session and the session's own elapsed time to be added on
// every save: playtime must never silently reset to zero or stand still.
func TestOnlineTimeAccruesAcrossSessions(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)

	const restoredPlaytime = 3600
	if _, err := srv.DB.Exec("UPDATE characters SET onlinetime = ? WHERE obj_Id = ?", restoredPlaytime, objID); err != nil {
		t.Fatalf("seed onlinetime: %v", err)
	}

	// A fresh connection re-loads the char list, so the seeded base is what
	// the entering session restores.
	c = srv.DialClient(t, srv.Account(), 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)

	persisted := persistedOnlineTime(t, srv, objID)
	if persisted < restoredPlaytime {
		t.Fatalf("onlinetime after enter world = %d, want >= %d (restored base kept)", persisted, restoredPlaytime)
	}

	// Stay in game just over a second so the session's own contribution to
	// the playtime total crosses the whole-second boundary.
	time.Sleep(1100 * time.Millisecond)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeLeaveWorld {
		t.Fatalf("logout opcode = %#x, want LeaveWorld (%#x)", reply[0], serverpackets.OpcodeLeaveWorld)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("post-logout opcode = %#x, want ActionFailed from detach's unconditional cast-stop ack (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
	c.ExpectClosed()

	final := persistedOnlineTime(t, srv, objID)
	if final <= persisted {
		t.Fatalf("onlinetime after logout = %d, want > %d (session elapsed time added)", final, persisted)
	}
}

func persistedOnlineTime(t *testing.T, srv *gameservertest.Server, objID int32) int64 {
	t.Helper()
	var seconds int64
	if err := srv.DB.QueryRow("SELECT onlinetime FROM characters WHERE obj_Id = ?", objID).Scan(&seconds); err != nil {
		t.Fatalf("read onlinetime: %v", err)
	}
	return seconds
}

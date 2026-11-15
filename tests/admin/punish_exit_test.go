package admin

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestChatBanTimeLeftStoredAtLogout pins the exit save of a timed
// punishment (Player.java:6288 Punishment.stopTask(true), then
// storeCharBase's punish_timer): the timer stops with the session and the
// time it had left is stored, so the next login resumes from there instead
// of the full term.
func TestChatBanTimeLeftStoredAtLogout(t *testing.T) {
	t.Parallel()
	srv := bootPunish(t)
	ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)
	const term = 10 * 60_000
	setPunishment(t, srv, ch.ID, punishChat, term)
	user := srv.DialClient(t, "player2", 1)
	if frames := burst(t, user); textAt(frames, "You are still chat banned for 10 minutes.") < 0 {
		t.Fatalf("burst = %x, want the 10 minutes reminder", testsupport.FrameOpcodes(frames))
	}

	srv.Advance(t, 2*time.Minute)
	user.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	if !user.AwaitClose(5 * time.Second) {
		t.Fatal("server kept the connection open after Logout")
	}
	var stored int64
	srv.AdvanceUntil(t, "time left stored", func() bool {
		_, stored = storedPunishment(t, srv, ch.ID)
		return stored != term
	})
	if level, _ := storedPunishment(t, srv, ch.ID); level != punishChat {
		t.Fatalf("stored punishment level after logout = %d, want %d", level, punishChat)
	}
	const left = term - 2*60_000
	if stored > left || stored < left-10_000 {
		t.Fatalf("stored punishment timer after logout = %d, want the %d left", stored, left)
	}

	user = srv.DialClient(t, "player2", 1)
	if frames := burst(t, user); textAt(frames, "You are still chat banned for 8 minutes.") < 0 {
		t.Fatalf("relog burst = %x, want the 8 minutes reminder", testsupport.FrameOpcodes(frames))
	}
}

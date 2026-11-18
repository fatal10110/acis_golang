package admin

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// punishExit leaves the world by one of the three exits that run the
// session's exit save (Player.java:6288 Punishment.stopTask(true)).
type punishExit struct {
	name  string
	leave func(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient)
}

var punishExits = []punishExit{
	{"restart", func(t *testing.T, _ *gameservertest.Server, c *testsupport.ScriptedClient) {
		c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
		for range 100 {
			if f := c.Read(); f == nil || f[0] == serverpackets.OpcodeCharSelectInfo {
				return
			}
		}
		t.Fatal("no CharSelectInfo after RequestRestart")
	}},
	{"lost connection", func(t *testing.T, _ *gameservertest.Server, c *testsupport.ScriptedClient) {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}},
}

// TestPunishmentTimeLeftStoredAtRestartAndLostConnection pins the exit
// save of a timed chat ban for the exits besides Logout
// (TestChatBanTimeLeftStoredAtLogout): the timer stops with the session,
// the time it had left is stored, and the next login resumes from there.
func TestPunishmentTimeLeftStoredAtRestartAndLostConnection(t *testing.T) {
	t.Parallel()
	for _, exit := range punishExits {
		t.Run(exit.name, func(t *testing.T) {
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
			exit.leave(t, srv, user)
			var stored int64
			srv.AdvanceUntil(t, "time left stored", func() bool {
				_, stored = storedPunishment(t, srv, ch.ID)
				return stored != term
			})
			if level, _ := storedPunishment(t, srv, ch.ID); level != punishChat {
				t.Fatalf("stored punishment level = %d, want %d", level, punishChat)
			}
			const left = term - 2*60_000
			if stored > left || stored < left-10_000 {
				t.Fatalf("stored punishment timer = %d, want the %d left", stored, left)
			}

			leftWorld(t, srv, "Player")
			user = srv.DialClient(t, "player2", 1)
			if frames := burst(t, user); textAt(frames, "You are still chat banned for 8 minutes.") < 0 {
				t.Fatalf("relog burst = %x, want the 8 minutes reminder", testsupport.FrameOpcodes(frames))
			}
		})
	}
}

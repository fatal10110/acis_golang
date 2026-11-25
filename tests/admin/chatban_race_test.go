package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestChatBanBySelectedSessionReportsChange pins //ban chat and //unban chat
// naming a character found by no player in the world, while a selection of
// it is under way (#5502): the session the selection starts gets the change
// as an online target does, checked against the jail it serves, and the GM
// is told what that check did instead of the offline answer.
func TestChatBanBySelectedSessionReportsChange(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ line, want string }{
		{"ban chat Player", "Player is already jailed, and can't receive another punishment."},
		{"unban chat Player", "Player isn't currently chat banned."},
	} {
		t.Run(tc.line, func(t *testing.T) {
			t.Parallel()
			srv := bootPunish(t)
			gm := srv.Client
			enterWorld(t, gm)
			ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)
			setPunishment(t, srv, ch.ID, punishJail, 0)
			user := srv.DialClient(t, "player2", 1)

			release := srv.HoldPersistenceLane(t, ch.ID)
			user.Send(encodeSelectSlot0())
			if got := srv.ReadQueued(t, user); len(got) != 0 {
				t.Fatalf("selection answered %x while its saves were held, want it waiting", testsupport.FrameOpcodes(got))
			}
			if got := exchange(t, gm, encodeBuildCmd(tc.line)); len(got) != 0 {
				t.Fatalf("answer while the selection waits = %x, want none before the session is checked", testsupport.FrameOpcodes(got))
			}
			release()
			srv.Settle(t)
			srv.FlushPersistence(t)
			user.ReadQueued()

			assertTexts(t, gm.ReadQueued(), tc.want)
			if level, _ := storedPunishment(t, srv, ch.ID); level != punishJail {
				t.Fatalf("stored punishment = %d, want the jail %d kept", level, punishJail)
			}
		})
	}
}

// TestChatBanOfLeavingTargetChecksStoredPunishment pins //ban chat and
// //unban chat reaching a player whose detach began (#5502): the change is
// stored after its logout saves only over the punishment stored then, not
// the one it left with, so a jail stored meanwhile through the same leaving
// path is neither lifted nor overwritten, and the GM is told what the store
// did.
func TestChatBanOfLeavingTargetChecksStoredPunishment(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		before []string
		lines  []string
		want   []string
		level  int
		timer  int64
	}{
		{
			name:   "unban chat over a jail stored meanwhile",
			before: []string{"ban chat Player"},
			lines:  []string{"jail", "unban chat Player"},
			want:   []string{"Player is jailed.", "Player isn't currently chat banned."},
			level:  punishJail,
		},
		{
			name:  "ban chat over a jail stored meanwhile",
			lines: []string{"jail", "ban chat Player 5"},
			want:  []string{"Player is jailed.", "Player is already jailed, and can't receive another punishment."},
			level: punishJail,
		},
		{
			name:   "unban chat",
			before: []string{"ban chat Player"},
			lines:  []string{"unban chat Player"},
			want:   []string{"Player's chat ban has been lifted."},
			level:  punishNone,
		},
		{
			name:  "ban chat",
			lines: []string{"ban chat Player 5"},
			want:  []string{"Player is chat banned for 5 minutes."},
			level: punishChat,
			timer: 5 * 60_000,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, userID, frames := commandsBehindLogout(t, tc.before, tc.lines...)
			got := texts(frames)
			at := -1
			for _, want := range tc.want {
				i := textAt(frames, want)
				if i <= at {
					t.Fatalf("GM texts = %v, want %q in order", got, tc.want)
				}
				at = i
			}
			if len(got) != len(tc.want) {
				t.Fatalf("GM texts = %v, want only %q", got, tc.want)
			}
			if level, timer := storedPunishment(t, srv, userID); level != tc.level || timer != tc.timer {
				t.Fatalf("stored punishment = %d timer %d, want %d timer %d", level, timer, tc.level, tc.timer)
			}
		})
	}
}

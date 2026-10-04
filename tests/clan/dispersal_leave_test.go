package clan

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// dissolutionRanOut marks Knights' dissolution as run out a day ago: the
// boot reschedules it a minute out.
func dissolutionRanOut() string {
	return `UPDATE clan_data SET dissolving_expiry_time = ` + strconv.FormatInt(time.Now().UnixMilli()-dayMs, 10) +
		` WHERE clan_id = ` + itoa(titleClanID)
}

// comeDue waits out the minute after boot, so Knights' dissolution comes
// due, and returns the frames c then gets.
func comeDue(t *testing.T, w *clanWorld, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	w.srv.Advance(t, time.Minute)
	frames := drainFrames(t, c)
	if _, ok := w.srv.Clans.Table().Get(titleClanID); ok {
		t.Fatal("Knights still stands after its dissolution came due")
	}
	return frames
}

// TestNobleKeepsTitleWhenClanDisperses has Knights disperse, its
// dissolution come due, with the titled recruit in the world: a noble
// keeps its title, in the UserInfo it gets and in its stored row, while a
// member that is no noble, the founder among them, loses it.
func TestNobleKeepsTitleWhenClanDisperses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		noble bool
		want  string
	}{
		{"noble", true, "Lord"},
		{"member", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootSeededClan(t, 3, 0, tc.noble, knightsSeed(t, 3, 0, dissolutionRanOut()))
			if !w.srv.DrivesClock() {
				t.Skip("the minute after boot is waited out on the test clock")
			}
			w.recruit(t)
			w.leader.Send(encodeRequestGiveNickName("Recruit", "Lord"))
			drainFrames(t, w.leader)
			drainFrames(t, w.member)
			w.leader.Send(encodeRequestGiveNickName("Founder", "Chief"))
			drainFrames(t, w.leader)
			drainFrames(t, w.member)
			if got := storedTitle(t, w, w.memberID); got != "Lord" {
				t.Fatalf("stored title before the dispersal = %q, want Lord", got)
			}

			if got := leaverTitle(t, comeDue(t, w, w.member)); got != tc.want {
				t.Fatalf("recruit's UserInfo title = %q, want %q", got, tc.want)
			}
			if got := storedTitle(t, w, w.memberID); got != tc.want {
				t.Fatalf("recruit's stored title = %q, want %q", got, tc.want)
			}
			if got := leaverTitle(t, drainFrames(t, w.leader)); got != "" {
				t.Fatalf("founder's UserInfo title = %q, want it cleared", got)
			}
			if got := storedTitle(t, w, w.leaderID); got != "" {
				t.Fatalf("founder's stored title = %q, want it cleared", got)
			}
		})
	}
}

// TestDispersalUnequipsOfflineMembersCastleCirclets has Knights disperse,
// its dissolution come due, with the recruit out of the world wearing the
// Circlet of Gludio: from a clan owning Gludio Castle the circlet is
// stored back in its inventory; from a clan owning none it stays worn.
func TestDispersalUnequipsOfflineMembersCastleCirclets(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		castle int
		want   string
	}{
		{"castle clan", 1, "INVENTORY"},
		{"clan without castle", 0, "PAPERDOLL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, circlet := bootCastleLeaver(t, tc.castle, circletOfGludio, knightsSeed(t, 5, tc.castle, dissolutionRanOut()))
			if !w.srv.DrivesClock() {
				t.Skip("the minute after boot is waited out on the test clock")
			}
			w.leaveWorld(t, w.member)
			drainFrames(t, w.leader)
			if loc := itemLoc(t, w, circlet); loc != "PAPERDOLL" {
				t.Fatalf("circlet before the dispersal = %s, want PAPERDOLL", loc)
			}

			comeDue(t, w, w.leader)
			if loc := itemLoc(t, w, circlet); loc != tc.want {
				t.Fatalf("offline member's circlet = %s, want %s", loc, tc.want)
			}
			if got := queryInt(t, w, `SELECT clanid FROM characters WHERE obj_Id = ?`, w.memberID); got != 0 {
				t.Fatalf("dispersed member's clanid = %d, want 0", got)
			}
		})
	}
}

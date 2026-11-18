package olympiad

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestHeroPageNameHoldsNoQueue holds the characters table while a player
// opens a hero's diary, whose page names the hero as its character is
// stored. While that read waits, the player's queue still runs work posted
// to it; once the lock lifts the one page arrives, naming the hero. Were the
// read made on the queue, the probe would wait behind it and the read would
// time out, sending no page.
func TestHeroPageNameHoldsNoQueue(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Login", 40, 0),
		gameservertest.WithWantChars(1),
		heroPageOptions(t),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithOlympiadSeed(seedStatements(t, []string{
			`INSERT INTO heroes (char_id, class_id, count, played, active, message) SELECT obj_Id, 88, 1, 1, 1, 'Hail' FROM characters WHERE char_name = 'Login'`,
		})),
	)
	c := srv.Client
	startInWorld(t, c)
	// The id is read before the lock: SoleObjectID reads the locked table.
	id := srv.SoleObjectID(t)

	release := srv.LockTable(t, "characters")
	c.Send(encodeHeroBypass("_diary?class=88&page=1"))
	srv.AwaitLockWait(t, "SELECT obj_Id, char_name")
	srv.AwaitQueueRuns(t, id)
	release()

	frames := drainFrames(t, c)
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("diary answered %d frames; want one NpcHtmlMessage", len(frames))
	}
	r := wire.NewReader(frames[0][1:])
	r.ReadInt32()
	if html := r.ReadString(); !strings.Contains(html, `History for :</font> Login<br>`) {
		t.Fatalf("diary page = %q, want it to name the hero Login", html)
	}
}

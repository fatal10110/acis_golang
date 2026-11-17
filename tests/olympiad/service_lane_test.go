package olympiad

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestOlympiadEndHoldsNoPlayerLane pins that the game-wide services write on
// a lane no player shares. With that lane stalled, as a slow database
// stalls it, the Olympiad's end still runs and queues its writes behind the
// stall; meanwhile a character whose object id is 0 mod 4 (the ids that
// used to share the services' lane) is selected, enters the world and has
// its autosave land, none of it waiting on the stalled writes. Once the
// stall clears the Olympiad's writes land too.
func TestOlympiadEndHoldsNoPlayerLane(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Lane", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithObjectIDsAfter(0x0FFFFFFF),
		gameservertest.WithOlympiadCompetition(time.Hour),
		gameservertest.WithOlympiadSeed(func(db *sql.DB) {
			for _, stmt := range []string{
				`UPDATE characters SET nobless = 1 WHERE char_name = 'Lane'`,
				`INSERT INTO olympiad_nobles SELECT obj_Id, 88, 70, 6, 4, 2, 0, 0 FROM characters WHERE char_name = 'Lane'`,
				`INSERT INTO server_memo (var, value) VALUES ('olympiad_cycle', '8')`,
			} {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatalf("seed %q: %v", stmt, err)
				}
			}
		}))
	objID := srv.SoleObjectID(t)
	if objID%4 != 0 {
		t.Fatalf("character object id = %d, want one that is 0 mod 4", objID)
	}

	release := srv.HoldPersistenceLane(t, persist.ServiceOwner)
	srv.Olympiad.SelectHeroes()
	srv.Settle(t)
	if got := srv.Olympiad.Period(); got != olympiad.Validation {
		t.Fatalf("period after the Olympiad's end = %v, want Validation", got)
	}
	if got := countRows(t, srv.DB, "olympiad_nobles_eom"); got != 0 {
		t.Fatalf("month's standings rows = %d while the services' lane is held, want 0: the Olympiad's end did not write on that lane", got)
	}

	// Selection waits for the character's queued saves; it must not wait
	// on the services' lane.
	startInWorld(t, srv.Client)
	srv.DamagePlayerHP(t, objID, 5)
	want := srv.PlayerCurrentHP(t, objID)
	srv.QueueAutosave()
	srv.AdvanceUntil(t, "the autosave of a character on lane 0", func() bool {
		return storedCurHP(t, srv.DB, objID) == want
	})
	if got := countRows(t, srv.DB, "olympiad_nobles_eom"); got != 0 {
		t.Fatalf("month's standings rows = %d before the services' lane was released, want 0", got)
	}

	release()
	srv.FlushPersistence(t)
	var points int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT olympiad_points FROM olympiad_nobles_eom WHERE char_id = ?", objID).Scan(&points); err != nil || points != 70 {
		t.Fatalf("month's standing = %d, %v; want 70 once the services' lane was released", points, err)
	}
}

// storedCurHP returns objID's stored current HP.
func storedCurHP(t *testing.T, db *sql.DB, objID int32) int {
	t.Helper()
	var hp int
	if err := db.QueryRowContext(context.Background(), "SELECT curHp FROM characters WHERE obj_Id = ?", objID).Scan(&hp); err != nil {
		t.Fatalf("read curHp: %v", err)
	}
	return hp
}

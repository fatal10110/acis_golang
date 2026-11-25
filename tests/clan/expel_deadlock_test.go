package clan

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestExpellingOfflineMemberSurvivesDeadlock expels an offline member while
// a character save holds its sponsor's row and then writes the member's
// own: the removal, which holds the member's row and waits on the
// sponsor's, deadlocks with it (#5504). The removal is rolled back as the
// victim and runs again, so the member still leaves the clan in the
// database, the sponsor's and the apprentice's links to it are cleared, the
// save's own writes stand, and nothing is logged as failed.
func TestExpellingOfflineMemberSurvivesDeadlock(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	w := bootClanWorld(t, 40, 0, 0, knightsSeed(t, 5, 0), gameservertest.WithCapturedLog())
	w.recruit(t)
	w.leaveWorld(t, w.member)
	sponsorID := w.srv.SeedCharacterFor(t, "player3", "Sponsor", 40, 0).ID
	pupilID := w.srv.SeedCharacterFor(t, "player4", "Pupil", 20, 0).ID
	seedStatements(t, w.srv.DB,
		`UPDATE characters SET sponsor = `+itoa(sponsorID)+` WHERE obj_Id = `+itoa(w.memberID),
		`UPDATE characters SET apprentice = `+itoa(w.memberID)+` WHERE obj_Id = `+itoa(sponsorID),
		`UPDATE characters SET sponsor = `+itoa(w.memberID)+` WHERE obj_Id = `+itoa(pupilID),
	)

	conn, err := w.srv.DB.Conn(ctx)
	if err != nil {
		t.Fatalf("open connection: %v", err)
	}
	defer conn.Close()
	save, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = save.Rollback() }()
	// The sponsor's skill rows make the save the heavier transaction, so
	// the deadlock detector rolls back the removal instead.
	for skill := int32(1); skill <= 32; skill++ {
		if _, err := save.ExecContext(ctx,
			`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, class_index, buff_index) VALUES (?, ?, 1, 0, ?)`,
			sponsorID, skill, skill); err != nil {
			t.Fatalf("save skill row %d: %v", skill, err)
		}
	}
	if _, err := save.ExecContext(ctx, `UPDATE characters SET exp = 7 WHERE obj_Id = ?`, sponsorID); err != nil {
		t.Fatalf("save sponsor row: %v", err)
	}

	drainFrames(t, w.leader)
	w.leader.Send(encodeRequestOustPledgeMember("Recruit"))
	// The removal holds the member's row once it waits on the sponsor's.
	awaitStatement(ctx, t, save, "UPDATE characters SET apprentice=0 WHERE apprentice")
	if _, err := save.ExecContext(ctx, `UPDATE characters SET exp = 9 WHERE obj_Id = ?`, w.memberID); err != nil {
		t.Fatalf("save member row: %v (want the removal, not the save, chosen as the deadlock victim)", err)
	}
	if err := save.Commit(); err != nil {
		t.Fatalf("commit save: %v", err)
	}
	drainFrames(t, w.leader)

	for _, c := range []struct {
		what  string
		query string
		id    int32
		want  int64
	}{
		{"member's clan", `SELECT clanid FROM characters WHERE obj_Id = ?`, w.memberID, 0},
		{"member's sponsor", `SELECT sponsor FROM characters WHERE obj_Id = ?`, w.memberID, 0},
		{"member's exp from the save", `SELECT exp FROM characters WHERE obj_Id = ?`, w.memberID, 9},
		{"sponsor's apprentice", `SELECT apprentice FROM characters WHERE obj_Id = ?`, sponsorID, 0},
		{"sponsor's exp from the save", `SELECT exp FROM characters WHERE obj_Id = ?`, sponsorID, 7},
		{"pupil's sponsor", `SELECT sponsor FROM characters WHERE obj_Id = ?`, pupilID, 0},
	} {
		if got := queryInt(t, w, c.query, c.id); got != c.want {
			t.Errorf("%s = %d, want %d", c.what, got, c.want)
		}
	}
	if log := w.srv.LogText(); strings.Contains(log, "clan: remove clan member") {
		t.Errorf("server logged a failed removal:\n%s", log)
	}
}

// awaitStatement returns once another connection to the server's database
// runs a statement starting with prefix. The process list is read live;
// the lock-wait tables are a snapshot refreshed only between idle reads.
func awaitStatement(ctx context.Context, t *testing.T, tx *sql.Tx, prefix string) {
	t.Helper()
	for {
		var running int
		err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM information_schema.PROCESSLIST
			 WHERE ID <> CONNECTION_ID() AND DB = DATABASE() AND INFO LIKE CONCAT(?, '%')`, prefix).Scan(&running)
		if ctx.Err() != nil {
			t.Fatalf("server never ran %q: %v", prefix, ctx.Err())
		}
		if err != nil {
			t.Fatalf("read process list: %v", err)
		}
		if running > 0 {
			return
		}
	}
}

package sql

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// Two saves for characters with nothing stored whose ids share an index gap
// each take a gap lock with their delete, then each insert waits on the
// other's: the database rolls one back as a deadlock victim (#5416). The
// victim's save is retried, so its rows still land and Replace reports
// success.
//
// The other save is a hand-driven transaction that has written more rows,
// so the database picks the Replace as the victim. Its insert returning
// before it commits proves the Replace's first attempt was rolled back:
// until then the Replace held the gap lock that insert waits on.
func TestSkillSaveStore_ReplaceRetriesDeadlockVictim(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// A fresh database: delete-marked rows a reused one still carries in
	// the index would move the gap boundaries this scenario relies on.
	db := sqltest.NewDB(t)
	store := NewSkillSaveStore(db)
	const (
		other      = int32(0x5E000000)
		otherClass = int32(0)
		saver      = other + 1
		classIndex = int32(0)
		heavy      = int32(other - 0x1000)
	)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open connection: %v", err)
	}
	defer conn.Close()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Rows far below the contended gap make this transaction the heavier
	// one, so the deadlock detector rolls back the Replace instead.
	for skill := int32(1); skill <= 32; skill++ {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, class_index, buff_index) VALUES (?, ?, 1, 0, ?)`,
			heavy, skill, skill); err != nil {
			t.Fatalf("insert weight row %d: %v", skill, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM character_skills_save WHERE char_obj_id = ? AND class_index = ?`, other, otherClass); err != nil {
		t.Fatalf("delete in contended gap: %v", err)
	}
	rows := []effect.SaveRow{
		{Skill: modelskill.Ref{ID: 1204, Level: 2}, EffectCount: 1, EffectCurTime: 300, RestoreType: effect.RestoreTypeEffect, BuffIndex: 1},
		{Skill: modelskill.Ref{ID: 1040, Level: 3}, EffectCount: 1, EffectCurTime: 120, RestoreType: effect.RestoreTypeEffect, BuffIndex: 2},
	}
	done := make(chan error, 1)
	go func() { done <- store.Replace(ctx, saver, classIndex, rows) }()

	// The Replace has taken its delete's gap lock once its insert runs.
	waitForInsert(ctx, t, tx, done)

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, class_index, buff_index) VALUES (?, 1, 1, ?, 1)`,
		other, otherClass); err != nil {
		t.Fatalf("contending insert: %v (want the Replace, not this transaction, chosen as the deadlock victim)", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("Replace() after losing a deadlock = %v, want nil after a retry", err)
	}
	got, err := store.ListByCharacter(ctx, saver, classIndex)
	if err != nil {
		t.Fatalf("ListByCharacter() unexpected error: %v", err)
	}
	if len(got) != len(rows) {
		t.Fatalf("rows after a retried Replace = %+v, want the %d saved rows", got, len(rows))
	}
	for i, row := range got {
		want := rows[i]
		want.ClassIndex = classIndex
		if row != want {
			t.Errorf("row %d after a retried Replace = %+v, want %+v", i, row, want)
		}
	}
}

// waitForInsert returns once another connection to this database is
// running an insert into character_skills_save. The process list is read
// live; the lock-wait tables are a snapshot refreshed only between idle
// reads, so a tight poll on them never sees a new waiter. It fails the
// test if the save under test returns first or ctx ends.
func waitForInsert(ctx context.Context, t *testing.T, tx *sql.Tx, done <-chan error) {
	t.Helper()
	for {
		select {
		case err := <-done:
			t.Fatalf("Replace() returned %v before inserting into the contended gap", err)
		default:
		}
		var running int
		err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM information_schema.PROCESSLIST
			 WHERE ID <> CONNECTION_ID() AND DB = DATABASE() AND INFO LIKE 'INSERT INTO character_skills_save%'`).Scan(&running)
		if ctx.Err() != nil {
			t.Fatalf("Replace() never reached its insert: %v", ctx.Err())
		}
		if err != nil {
			t.Fatalf("read process list: %v", err)
		}
		if running > 0 {
			return
		}
	}
}

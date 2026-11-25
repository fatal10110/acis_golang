package sql

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
)

// An offline member's removal clears its own row, then the links other
// characters hold to it. A character save that has locked the apprentice's
// row and then writes the member's row deadlocks with it (#5504). The
// removal is the victim and runs again, so the member still leaves with
// every link to it cleared, and the save's own writes stand.
//
// The save is a hand-driven transaction that has written more rows, so the
// database picks the removal as the victim. Its update of the member's row
// returning before it commits proves the removal's first attempt was rolled
// back: until then the removal held the member's row.
func TestClanStore_RemoveMembershipRetriesDeadlockVictim(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db := sqltest.NewDB(t)
	if err := EnsureIndexes(ctx, db); err != nil {
		t.Fatalf("add indexes: %v", err)
	}
	store := NewClanStore(db)
	const (
		member     = int32(0x5E000010)
		sponsor    = member + 1
		apprentice = member + 2
	)
	for _, row := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO characters (account_name, obj_Id, char_name, clanid, title, sponsor) VALUES ('a', ?, 'Member', 600, 'Knight', ?)`, []any{member, sponsor}},
		{`INSERT INTO characters (account_name, obj_Id, char_name, clanid, apprentice) VALUES ('b', ?, 'Sponsor', 600, ?)`, []any{sponsor, member}},
		{`INSERT INTO characters (account_name, obj_Id, char_name, clanid, sponsor) VALUES ('c', ?, 'Pupil', 600, ?)`, []any{apprentice, member}},
	} {
		if _, err := db.ExecContext(ctx, row.query, row.args...); err != nil {
			t.Fatalf("seed character: %v", err)
		}
	}

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

	// The sponsor's skill rows make this save the heavier transaction, so
	// the deadlock detector rolls back the removal instead.
	for skill := int32(1); skill <= 32; skill++ {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, class_index, buff_index) VALUES (?, ?, 1, 0, ?)`,
			sponsor, skill, skill); err != nil {
			t.Fatalf("insert weight row %d: %v", skill, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE characters SET exp = 7 WHERE obj_Id = ?`, sponsor); err != nil {
		t.Fatalf("lock sponsor row: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- store.RemoveMembership(ctx, clan.RemovalRow{ObjectID: member, JoinExpiry: 1234}) }()

	// The removal holds the member's row once it waits on the sponsor's.
	waitForStatement(ctx, t, tx, done, "UPDATE characters SET apprentice=0 WHERE apprentice")

	if _, err := tx.ExecContext(ctx, `UPDATE characters SET exp = 9 WHERE obj_Id = ?`, member); err != nil {
		t.Fatalf("contending update: %v (want the removal, not this transaction, chosen as the deadlock victim)", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("RemoveMembership() after losing a deadlock = %v, want nil after a retry", err)
	}
	for _, c := range []struct {
		query string
		id    int32
		want  string
	}{
		{`SELECT CONCAT_WS(',', clanid, title, clan_join_expiry_time, sponsor, exp) FROM characters WHERE obj_Id = ?`, member, "0,,1234,0,9"},
		{`SELECT CONCAT_WS(',', clanid, apprentice, exp) FROM characters WHERE obj_Id = ?`, sponsor, "600,0,7"},
		{`SELECT CONCAT_WS(',', clanid, sponsor) FROM characters WHERE obj_Id = ?`, apprentice, "600,0"},
	} {
		var got string
		if err := db.QueryRowContext(ctx, c.query, c.id).Scan(&got); err != nil {
			t.Fatalf("read %d: %v", c.id, err)
		}
		if got != c.want {
			t.Errorf("row %d after a retried removal = %q, want %q", c.id, got, c.want)
		}
	}
}

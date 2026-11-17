package dbtest

import (
	"context"
	"errors"
	"testing"
)

// TestFaultFailNthFailsOnlyThatStatement proves FailNth fails exactly the
// n-th matching statement, with or without arguments and inside a
// transaction, counts each statement once, and lets every other statement
// through, so a suite can fail one write in the middle of a chain.
func TestFaultFailNthFailsOnlyThatStatement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB(t, "CREATE TABLE rows_t (id INT PRIMARY KEY)", "CREATE TABLE other_t (id INT PRIMARY KEY)")
	fault := &Fault{}
	faulty := fault.Wrap(t, db)

	fault.FailNth("INSERT INTO rows_t", 2)
	if _, err := faulty.ExecContext(ctx, "INSERT INTO other_t VALUES (1)"); err != nil {
		t.Fatalf("unmatched insert: %v", err)
	}
	if _, err := faulty.ExecContext(ctx, "INSERT INTO rows_t VALUES (?)", 1); err != nil {
		t.Fatalf("first matching insert: %v", err)
	}
	tx, err := faulty.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO rows_t VALUES (2)"); !errors.Is(err, ErrFault) {
		t.Fatalf("second matching insert = %v, want ErrFault", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO rows_t VALUES (?)", 3); err != nil {
		t.Fatalf("third matching insert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var ids []int
	rows, err := db.QueryContext(ctx, "SELECT id FROM rows_t ORDER BY id")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 3 {
		t.Fatalf("rows_t ids = %v, want [1 3]", ids)
	}
}

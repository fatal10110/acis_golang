package dbtest

import (
	"context"
	"errors"
	"testing"
)

// TestFaultFailsEveryOperationWhileArmed proves an armed Fault takes the
// wrapped handle's database away from every kind of use, including a
// statement prepared and a transaction begun before it was armed, while the
// handle it wraps keeps working; and that disarming brings the wrapped handle
// back on its pooled connections.
func TestFaultFailsEveryOperationWhileArmed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := NewDB(t, "CREATE TABLE rows_t (id INT PRIMARY KEY)")
	fault := &Fault{}
	faulty := fault.Wrap(t, db)

	if _, err := faulty.ExecContext(ctx, "INSERT INTO rows_t VALUES (?)", 1); err != nil {
		t.Fatalf("disarmed insert: %v", err)
	}
	stmt, err := faulty.PrepareContext(ctx, "INSERT INTO rows_t VALUES (?)")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer stmt.Close()
	tx, err := faulty.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO rows_t VALUES (?)", 2); err != nil {
		t.Fatalf("insert in transaction: %v", err)
	}

	fault.Arm()
	if err := tx.Commit(); !errors.Is(err, ErrFault) {
		t.Fatalf("armed commit = %v, want ErrFault", err)
	}
	checks := map[string]func() error{
		"exec": func() error {
			_, err := faulty.ExecContext(ctx, "INSERT INTO rows_t VALUES (?)", 3)
			return err
		},
		"query": func() error {
			rows, err := faulty.QueryContext(ctx, "SELECT id FROM rows_t")
			if err == nil {
				rows.Close()
			}
			return err
		},
		"prepared exec": func() error {
			_, err := stmt.ExecContext(ctx, 4)
			return err
		},
		"begin": func() error {
			tx, err := faulty.BeginTx(ctx, nil)
			if err == nil {
				_ = tx.Rollback()
			}
			return err
		},
		"ping": func() error { return faulty.PingContext(ctx) },
	}
	for name, check := range checks {
		if err := check(); !errors.Is(err, ErrFault) {
			t.Errorf("armed %s = %v, want ErrFault", name, err)
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM rows_t").Scan(&count); err != nil {
		t.Fatalf("wrapped handle while armed: %v", err)
	}
	if count != 1 {
		t.Fatalf("rows while armed = %d, want 1: an armed write landed", count)
	}

	fault.Disarm()
	if _, err := stmt.ExecContext(ctx, 5); err != nil {
		t.Fatalf("disarmed prepared exec: %v", err)
	}
	if err := faulty.QueryRowContext(ctx, "SELECT COUNT(*) FROM rows_t").Scan(&count); err != nil {
		t.Fatalf("disarmed query: %v", err)
	}
	if count != 2 {
		t.Fatalf("rows after disarm = %d, want 2", count)
	}
}

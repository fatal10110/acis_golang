package sql

import (
	"context"
	"errors"

	"github.com/go-sql-driver/mysql"
)

// txAttempts bounds how many times retryLockConflict runs a transaction
// the database keeps rolling back for a lock conflict.
const txAttempts = 5

// retryLockConflict runs run, a whole transaction from begin to commit, and
// runs it again while the database refuses it for a lock conflict: a
// deadlock (InnoDB rolls the victim back whole) or a lock wait timeout (run
// rolls back the rest). Two saves for different owners whose keys share an
// index gap can deadlock on their delete-then-insert, and dropping the
// loser would lose the state it carried. Any other error, ctx ending, or
// the last attempt's conflict is returned as is.
func retryLockConflict(ctx context.Context, run func() error) error {
	var err error
	for range txAttempts {
		if err = run(); err == nil || !lockConflict(err) || ctx.Err() != nil {
			return err
		}
	}
	return err
}

// lockConflict reports whether err is the database rolling a statement or
// its transaction back for a lock conflict, which the same transaction can
// clear by running again.
func lockConflict(err error) bool {
	var me *mysql.MySQLError
	if !errors.As(err, &me) {
		return false
	}
	switch me.Number {
	case 1205, // lock wait timeout exceeded
		1213: // deadlock found
		return true
	}
	return false
}

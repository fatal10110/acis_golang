package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// secondaryIndexes are the indexes the server's reads need beyond the
// shipped schema, which keys items and characters by object id only:
// without them every read of a character's items, of an account's
// characters or of a character by name scans the whole table. Each index is
// named after its column.
var secondaryIndexes = []struct{ table, column string }{
	{"items", "owner_id"},
	{"characters", "account_name"},
	{"characters", "char_name"},
}

// EnsureIndexes adds each secondary index db lacks. A column that already
// leads an index, whatever its name, counts as indexed, so a database that
// has one, added at an earlier boot or by hand, is not altered again. The
// step only ever adds an index; it never drops or changes one. Adding one to
// a large table rebuilds it once, at the first boot that finds it missing.
// It refuses a database whose name columns tell letter cases apart
// (caseFoldedColumns), so no index is added to one.
func EnsureIndexes(ctx context.Context, db *sql.DB) error {
	if err := checkCaseFolded(ctx, db); err != nil {
		return err
	}
	for _, ix := range secondaryIndexes {
		var n int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM information_schema.statistics
			 WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ? AND seq_in_index = 1`,
			ix.table, ix.column).Scan(&n); err != nil {
			return fmt.Errorf("read indexes of %s: %w", ix.table, err)
		}
		if n > 0 {
			continue
		}
		stmt := fmt.Sprintf("CREATE INDEX IF NOT EXISTS `%s` ON `%s` (`%s`)", ix.column, ix.table, ix.column)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("add index %s(%s): %w", ix.table, ix.column, err)
		}
	}
	return nil
}

// caseFoldedColumns are the name columns matched by their own comparison,
// which must ignore letter case: character and account names are
// case-insensitive, so a name differing only in case is the same name.
var caseFoldedColumns = []struct{ table, column string }{
	{"characters", "char_name"},
	{"characters", "account_name"},
}

// checkCaseFolded refuses a database where a caseFoldedColumns column has a
// collation that does not fold case (one not ending in _ci).
func checkCaseFolded(ctx context.Context, db *sql.DB) error {
	for _, col := range caseFoldedColumns {
		var collation sql.NullString
		if err := db.QueryRowContext(ctx,
			`SELECT collation_name FROM information_schema.columns
			 WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			col.table, col.column).Scan(&collation); err != nil {
			return fmt.Errorf("read collation of %s.%s: %w", col.table, col.column, err)
		}
		if !strings.HasSuffix(collation.String, "_ci") {
			return fmt.Errorf("%s.%s has collation %q, which tells letter cases apart; names need a case-insensitive (_ci) collation",
				col.table, col.column, collation.String)
		}
	}
	return nil
}

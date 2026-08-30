// Package dbtest connects integration tests to the single shared MariaDB
// instance started once via `make test-db-up` (docker-compose.test.yml),
// instead of each test package or test function starting its own
// testcontainer. Callers get their own uniquely named database on that
// instance so tests stay isolated from each other without needing a
// container per package.
package dbtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
)

const defaultRootDSN = "root:test@tcp(127.0.0.1:3307)/"

func rootDSN() string {
	if dsn := os.Getenv("ACIS_TEST_MARIADB_DSN"); dsn != "" {
		return dsn
	}
	return defaultRootDSN
}

// NewName returns a fresh, unique database name.
func NewName() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "acis_test_" + hex.EncodeToString(b)
}

// Open creates database name on the shared MariaDB instance, applies
// schemaStmts to it, and returns a pool connected to it. The caller owns its
// lifecycle (Drop + db.Close).
func Open(ctx context.Context, name string, schemaStmts ...string) (*sql.DB, error) {
	root, err := sql.Open("mysql", rootDSN())
	if err != nil {
		return nil, fmt.Errorf("open root db: %w", err)
	}
	defer root.Close()
	if _, err := root.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		return nil, fmt.Errorf("create database %s: %w", name, err)
	}

	db, err := sql.Open("mysql", rootDSN()+name)
	if err != nil {
		return nil, fmt.Errorf("open db %s: %w", name, err)
	}
	for _, stmt := range schemaStmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply schema to %s: %w", name, err)
		}
	}
	return db, nil
}

// Drop removes database name from the shared instance.
func Drop(name string) {
	root, err := sql.Open("mysql", rootDSN())
	if err != nil {
		return
	}
	defer root.Close()
	root.Exec("DROP DATABASE IF EXISTS `" + name + "`")
}

// NewDB creates a fresh database, applies schemaStmts, and returns a pool
// connected to it. The database is dropped and the pool closed when tb's
// test completes.
func NewDB(tb testing.TB, schemaStmts ...string) *sql.DB {
	tb.Helper()
	name := NewName()
	db, err := Open(context.Background(), name, schemaStmts...)
	if err != nil {
		tb.Fatalf("%v", err)
	}
	tb.Cleanup(func() {
		db.Close()
		Drop(name)
	})
	return db
}

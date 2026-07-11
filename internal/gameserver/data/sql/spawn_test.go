package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

func TestSpawnStoreSaveStatesUsesSingleTransaction(t *testing.T) {
	rec := &spawnStoreRecorder{}
	db := sql.OpenDB(spawnStoreConnector{rec: rec})
	t.Cleanup(func() { _ = db.Close() })
	store := NewSpawnStore(db)

	err := store.SaveStates(context.Background(), map[string]*spawn.State{
		"alive": {
			Name:        "alive",
			Status:      spawn.StatusAlive,
			CurrentHP:   120,
			CurrentMP:   30,
			Location:    location.Location{X: 1, Y: 2, Z: 3},
			Heading:     4,
			DBValue:     5,
			RespawnTime: 6,
		},
		"dead": {
			Name:        "dead",
			Status:      spawn.StatusDead,
			RespawnTime: 9_000,
		},
		"new": spawn.NewState("new"),
	})
	if err != nil {
		t.Fatalf("SaveStates() error = %v", err)
	}

	if rec.begins != 1 || rec.commits != 1 || rec.rollbacks != 0 {
		t.Fatalf("transaction counts: begin=%d commit=%d rollback=%d, want 1/1/0", rec.begins, rec.commits, rec.rollbacks)
	}
	if rec.deletes != 1 {
		t.Fatalf("DELETE count = %d, want 1", rec.deletes)
	}
	if rec.inserts != 2 {
		t.Fatalf("INSERT count = %d, want 2", rec.inserts)
	}
}

type spawnStoreRecorder struct {
	begins, commits, rollbacks int
	deletes, inserts           int
}

type spawnStoreConnector struct {
	rec *spawnStoreRecorder
}

func (c spawnStoreConnector) Connect(context.Context) (driver.Conn, error) {
	return &spawnStoreConn{rec: c.rec}, nil
}

func (c spawnStoreConnector) Driver() driver.Driver {
	return spawnStoreDriver{}
}

type spawnStoreDriver struct{}

func (spawnStoreDriver) Open(string) (driver.Conn, error) {
	return nil, driver.ErrSkip
}

type spawnStoreConn struct {
	rec *spawnStoreRecorder
}

func (c *spawnStoreConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c *spawnStoreConn) PrepareContext(_ context.Context, query string) (driver.Stmt, error) {
	return &spawnStoreStmt{rec: c.rec, query: query}, nil
}

func (c *spawnStoreConn) Close() error { return nil }

func (c *spawnStoreConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *spawnStoreConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	c.rec.begins++
	return &spawnStoreTx{rec: c.rec}, nil
}

func (c *spawnStoreConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.HasPrefix(strings.ToUpper(query), "DELETE ") {
		c.rec.deletes++
	}
	return driver.RowsAffected(0), nil
}

type spawnStoreTx struct {
	rec *spawnStoreRecorder
}

func (tx *spawnStoreTx) Commit() error {
	tx.rec.commits++
	return nil
}

func (tx *spawnStoreTx) Rollback() error {
	tx.rec.rollbacks++
	return nil
}

type spawnStoreStmt struct {
	rec   *spawnStoreRecorder
	query string
}

func (s *spawnStoreStmt) Close() error { return nil }

func (s *spawnStoreStmt) NumInput() int { return -1 }

func (s *spawnStoreStmt) Exec([]driver.Value) (driver.Result, error) {
	if strings.HasPrefix(strings.ToUpper(s.query), "INSERT ") {
		s.rec.inserts++
	}
	return driver.RowsAffected(1), nil
}

func (s *spawnStoreStmt) ExecContext(_ context.Context, _ []driver.NamedValue) (driver.Result, error) {
	if strings.HasPrefix(strings.ToUpper(s.query), "INSERT ") {
		s.rec.inserts++
	}
	return driver.RowsAffected(1), nil
}

func (s *spawnStoreStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, io.EOF
}

package dbtest

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// PoolConfig describes the databases a Pool hands out.
type PoolConfig struct {
	// Schema is applied once, when the pool creates a database.
	Schema []string
	// Seed restores default rows. It runs after Schema and again after
	// every test's cleanup, so each test starts from the same data.
	Seed []string
}

// Pool hands tests databases on the shared instance that all carry the same
// schema. A test checks one out and it is emptied and returned when the test
// completes, so parallel tests each hold their own database while
// sequential tests reuse one instead of paying for CREATE DATABASE and the
// schema every time. Repeated calls from the same test return the same
// database; a t.Run subtest is a different test and gets a different
// database from its parent.
//
// A package using a Pool must run its tests through Main, which drops every
// pooled database once the package's tests finish. Tables are emptied in no
// particular order, so a schema with cross-table foreign keys would fail its
// cleanup.
type Pool struct {
	cfg PoolConfig

	mu   sync.Mutex
	free []*pooledDB
	all  []*pooledDB
	held map[testing.TB]*pooledDB
}

type pooledDB struct {
	db   *sql.DB
	name string
	// tables lists the database's tables, split by how they are emptied.
	deleteTables, truncateTables []string
}

var (
	poolsMu     sync.Mutex
	pools       []*Pool
	mainRunning atomic.Bool
)

// NewPool returns a Pool for cfg. It connects to nothing until a test
// checks out a database, so it is safe to create as a package variable.
func NewPool(cfg PoolConfig) *Pool {
	p := &Pool{cfg: cfg, held: map[testing.TB]*pooledDB{}}
	poolsMu.Lock()
	pools = append(pools, p)
	poolsMu.Unlock()
	return p
}

// Main runs a package's tests and then drops every database created by any
// Pool in the test binary. Every package that uses a Pool must call it:
//
//	func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }
func Main(m *testing.M) int {
	mainRunning.Store(true)
	code := m.Run()
	poolsMu.Lock()
	defer poolsMu.Unlock()
	for _, p := range pools {
		p.close()
	}
	return code
}

// DB returns tb's database, checking one out of the pool on first use.
func (p *Pool) DB(tb testing.TB) *sql.DB {
	tb.Helper()
	if !mainRunning.Load() {
		tb.Fatal("dbtest: pooled databases need the package's TestMain to call dbtest.Main")
	}
	p.mu.Lock()
	if d, ok := p.held[tb]; ok {
		p.mu.Unlock()
		return d.db
	}
	var d *pooledDB
	if n := len(p.free); n > 0 {
		d = p.free[n-1]
		p.free = p.free[:n-1]
	}
	p.mu.Unlock()

	if d == nil {
		var err error
		if d, err = p.create(context.Background()); err != nil {
			tb.Fatalf("pooled test db: %v", err)
		}
		p.mu.Lock()
		p.all = append(p.all, d)
		p.mu.Unlock()
	}
	p.mu.Lock()
	p.held[tb] = d
	p.mu.Unlock()

	tb.Cleanup(func() {
		if err := p.reset(context.Background(), d); err != nil {
			tb.Fatalf("reset pooled test db: %v", err)
		}
		p.mu.Lock()
		delete(p.held, tb)
		p.free = append(p.free, d)
		p.mu.Unlock()
	})
	return d.db
}

func (p *Pool) create(ctx context.Context) (*pooledDB, error) {
	name := NewName()
	db, err := Open(ctx, name, append(append([]string(nil), p.cfg.Schema...), p.cfg.Seed...)...)
	if err != nil {
		return nil, err
	}
	d := &pooledDB{db: db, name: name}
	rows, err := db.QueryContext(ctx, "SELECT table_name, auto_increment IS NOT NULL "+
		"FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'")
	if err != nil {
		db.Close()
		Drop(name)
		return nil, fmt.Errorf("list tables of %s: %w", name, err)
	}
	defer rows.Close()
	for rows.Next() {
		var table string
		var autoIncrement bool
		if err := rows.Scan(&table, &autoIncrement); err != nil {
			db.Close()
			Drop(name)
			return nil, fmt.Errorf("list tables of %s: %w", name, err)
		}
		if autoIncrement {
			d.truncateTables = append(d.truncateTables, table)
		} else {
			d.deleteTables = append(d.deleteTables, table)
		}
	}
	if err := rows.Err(); err != nil {
		db.Close()
		Drop(name)
		return nil, fmt.Errorf("list tables of %s: %w", name, err)
	}
	return d, nil
}

// reset empties d and reapplies the seed rows. Tables are emptied with
// DELETE rather than TRUNCATE: TRUNCATE is InnoDB DDL that recreates the
// tablespace under the server-wide dictionary lock, so cleanups from every
// parallel test and concurrent `go test` run on the shared instance would
// queue behind one another. Only tables with an AUTO_INCREMENT column are
// truncated, because DELETE does not reset the counter.
func (p *Pool) reset(ctx context.Context, d *pooledDB) error {
	for _, table := range d.deleteTables {
		if _, err := d.db.ExecContext(ctx, "DELETE FROM `"+table+"`"); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}
	for _, table := range d.truncateTables {
		if _, err := d.db.ExecContext(ctx, "TRUNCATE TABLE `"+table+"`"); err != nil {
			return fmt.Errorf("truncate %s: %w", table, err)
		}
	}
	for _, stmt := range p.cfg.Seed {
		if _, err := d.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("reseed: %w", err)
		}
	}
	return nil
}

func (p *Pool) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, d := range p.all {
		d.db.Close()
		Drop(d.name)
	}
	p.all, p.free = nil, nil
}

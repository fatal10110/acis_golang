package dbtest

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// ErrFault is what every operation on a handle of an armed Fault fails with.
var ErrFault = errors.New("dbtest: database fault")

// Fault takes a database away from the code under test without touching the
// shared instance: a handle Wrap returns fails every connect, statement,
// transaction and ping while the fault is armed, and works again once it is
// disarmed. The handle it wraps is never affected, so a suite seeds and
// asserts rows through it while the server's own handle is down.
//
// Arm and Disarm are safe to call while other goroutines use the handle.
type Fault struct {
	armed atomic.Bool
}

// Arm makes every later operation on f's handles fail until Disarm.
func (f *Fault) Arm() { f.armed.Store(true) }

// Disarm lets f's handles reach the database again.
func (f *Fault) Disarm() { f.armed.Store(false) }

// Wrap returns a second handle on db's database that fails while f is armed.
// The handle is closed when tb completes.
func (f *Fault) Wrap(tb testing.TB, db *sql.DB) *sql.DB {
	tb.Helper()
	var name string
	if err := db.QueryRowContext(context.Background(), "SELECT DATABASE()").Scan(&name); err != nil {
		tb.Fatalf("dbtest: name the wrapped database: %v", err)
	}
	cfg, err := mysql.ParseDSN(rootDSN() + name)
	if err != nil {
		tb.Fatalf("dbtest: %v", err)
	}
	inner, err := mysql.NewConnector(cfg)
	if err != nil {
		tb.Fatalf("dbtest: %v", err)
	}
	wrapped := sql.OpenDB(faultConnector{inner: inner, fault: f})
	tb.Cleanup(func() { _ = wrapped.Close() })
	return wrapped
}

type faultConnector struct {
	inner driver.Connector
	fault *Fault
}

// driverConn is every optional interface the MySQL driver's connections
// implement, so the wrapper forwards each of them rather than letting
// database/sql fall back to slower or differently converting paths.
type driverConn interface {
	driver.Conn
	driver.ConnBeginTx
	driver.ConnPrepareContext
	driver.ExecerContext
	driver.QueryerContext
	driver.Pinger
	driver.SessionResetter
	driver.Validator
	driver.NamedValueChecker
}

func (c faultConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if c.fault.armed.Load() {
		return nil, ErrFault
	}
	conn, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	full, ok := conn.(driverConn)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("dbtest: driver connection %T lacks an interface the fault wrapper forwards", conn)
	}
	return faultConn{inner: full, fault: c.fault}, nil
}

func (c faultConnector) Driver() driver.Driver { return c.inner.Driver() }

type faultConn struct {
	inner driverConn
	fault *Fault
}

func (c faultConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c faultConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if c.fault.armed.Load() {
		return nil, ErrFault
	}
	st, err := c.inner.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	full, ok := st.(driverStmt)
	if !ok {
		_ = st.Close()
		return nil, fmt.Errorf("dbtest: driver statement %T lacks an interface the fault wrapper forwards", st)
	}
	return faultStmt{inner: full, fault: c.fault}, nil
}

func (c faultConn) Close() error { return c.inner.Close() }

func (c faultConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c faultConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if c.fault.armed.Load() {
		return nil, ErrFault
	}
	tx, err := c.inner.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return faultTx{inner: tx, fault: c.fault}, nil
}

func (c faultConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.fault.armed.Load() {
		return nil, ErrFault
	}
	return c.inner.ExecContext(ctx, query, args)
}

func (c faultConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if c.fault.armed.Load() {
		return nil, ErrFault
	}
	return c.inner.QueryContext(ctx, query, args)
}

func (c faultConn) Ping(ctx context.Context) error {
	if c.fault.armed.Load() {
		return ErrFault
	}
	return c.inner.Ping(ctx)
}

// ResetSession drops a pooled connection while armed, so database/sql asks
// the connector for a new one, which fails too.
func (c faultConn) ResetSession(ctx context.Context) error {
	if c.fault.armed.Load() {
		return driver.ErrBadConn
	}
	return c.inner.ResetSession(ctx)
}

func (c faultConn) IsValid() bool { return !c.fault.armed.Load() && c.inner.IsValid() }

func (c faultConn) CheckNamedValue(v *driver.NamedValue) error { return c.inner.CheckNamedValue(v) }

// driverStmt is the context-aware statement the MySQL driver prepares.
type driverStmt interface {
	driver.Stmt
	driver.StmtExecContext
	driver.StmtQueryContext
}

// faultStmt fails a statement prepared before the fault was armed.
type faultStmt struct {
	inner driverStmt
	fault *Fault
}

func (s faultStmt) Close() error  { return s.inner.Close() }
func (s faultStmt) NumInput() int { return s.inner.NumInput() }

func (s faultStmt) Exec(args []driver.Value) (driver.Result, error) {
	if s.fault.armed.Load() {
		return nil, ErrFault
	}
	return s.inner.Exec(args) //nolint:staticcheck // forwards the legacy entry point database/sql may still call
}

func (s faultStmt) Query(args []driver.Value) (driver.Rows, error) {
	if s.fault.armed.Load() {
		return nil, ErrFault
	}
	return s.inner.Query(args) //nolint:staticcheck // forwards the legacy entry point database/sql may still call
}

func (s faultStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if s.fault.armed.Load() {
		return nil, ErrFault
	}
	return s.inner.ExecContext(ctx, args)
}

func (s faultStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if s.fault.armed.Load() {
		return nil, ErrFault
	}
	return s.inner.QueryContext(ctx, args)
}

// faultTx fails the commit of a transaction begun before the fault was armed,
// rolling it back so none of its writes land.
type faultTx struct {
	inner driver.Tx
	fault *Fault
}

func (t faultTx) Commit() error {
	if t.fault.armed.Load() {
		_ = t.inner.Rollback()
		return ErrFault
	}
	return t.inner.Commit()
}

func (t faultTx) Rollback() error { return t.inner.Rollback() }

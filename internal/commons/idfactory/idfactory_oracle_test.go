package idfactory

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
)

// recorder is a database driver that answers every statement with an empty
// result and remembers the statement texts it was given.
type recorder struct {
	mu   sync.Mutex
	seen []string
}

func (r *recorder) record(kind, query string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, kind+" "+query)
}

type recorderConn struct{ r *recorder }

func (c recorderConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c recorderConn) Close() error                        { return nil }
func (c recorderConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (c recorderConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.r.record("exec", query)
	return driver.RowsAffected(0), nil
}

func (c recorderConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.r.record("query", query)
	return emptyRows{}, nil
}

type emptyRows struct{}

func (emptyRows) Columns() []string         { return []string{"id"} }
func (emptyRows) Close() error              { return nil }
func (emptyRows) Next([]driver.Value) error { return io.EOF }

type recorderDriver struct{ r *recorder }

func (d recorderDriver) Open(string) (driver.Conn, error) { return recorderConn{d.r}, nil }

// TestBootStatementsMatchReferenceProbe replays testdata/statements.golden,
// the output of testdata/oracle/IdFactoryProbe.java: every statement the
// reference IdFactory runs at boot, in order. New must run the same
// statements in the same order (the reference's trailing semicolons aside).
func TestBootStatementsMatchReferenceProbe(t *testing.T) {
	data, err := os.ReadFile("testdata/statements.golden")
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		kind, sqlText, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("malformed golden line %q", line)
		}
		if kind == "update" || kind == "prepare" {
			kind = "exec"
		}
		want = append(want, kind+" "+strings.TrimSuffix(sqlText, ";"))
	}
	if len(want) < 50 {
		t.Fatalf("golden has %d statements, want the whole boot pass", len(want))
	}

	rec := &recorder{}
	db := sql.OpenDB(connectorFor(rec))
	defer db.Close()
	if _, err := New(context.Background(), db, zerolog.Nop()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rec.seen, want) {
		for i := range max(len(rec.seen), len(want)) {
			var g, w string
			if i < len(rec.seen) {
				g = rec.seen[i]
			}
			if i < len(want) {
				w = want[i]
			}
			if g != w {
				t.Fatalf("statement %d = %q, want %q (%d run, %d expected)", i+1, g, w, len(rec.seen), len(want))
			}
		}
	}
}

type connector struct{ r *recorder }

func connectorFor(r *recorder) driver.Connector { return connector{r} }

func (c connector) Connect(context.Context) (driver.Conn, error) { return recorderConn{c.r}, nil }
func (c connector) Driver() driver.Driver                        { return recorderDriver{c.r} }

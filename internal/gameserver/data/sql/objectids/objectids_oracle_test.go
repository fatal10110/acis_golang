package objectids

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

type recorderConn struct{ *recorder }

func (c recorderConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c recorderConn) Close() error                        { return nil }
func (c recorderConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (c recorderConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.record("exec", query)
	return driver.RowsAffected(0), nil
}

func (c recorderConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.record("query", query)
	return emptyRows{}, nil
}

type emptyRows struct{}

func (emptyRows) Columns() []string         { return []string{"id"} }
func (emptyRows) Close() error              { return nil }
func (emptyRows) Next([]driver.Value) error { return io.EOF }

// TestBootStatementsMatchReferenceProbe replays testdata/statements.golden,
// the output of testdata/oracle/IdFactoryProbe.java: every statement the
// reference IdFactory runs at boot, in order. NewAllocator must run the same
// statements in the same order (the reference's trailing semicolons aside),
// plus the owner-decided #4676 additions, each at its stated place.
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
	for _, add := range issue4676Additions {
		at := slices.Index(want, add.after)
		if at < 0 {
			t.Fatalf("#4676 addition anchor %q is not in the golden", add.after)
		}
		want = slices.Insert(want, at+1, add.stmts...)
	}

	rec := &recorder{}
	db := sql.OpenDB(connector{rec})
	defer db.Close()
	if _, err := NewAllocator(context.Background(), db, zerolog.Nop()); err != nil {
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

// issue4676Additions are the statements NewAllocator runs beyond the reference
// probe's, by owner decision on #4676: the boot cleanup removes a missing
// character's bookmarks, schemes and marriage, and the id scan counts the
// retired ids of deleted characters, creating their table first. Each runs
// right after the statement named by after.
var issue4676Additions = []struct {
	after string
	stmts []string
}{
	{
		after: "exec DELETE FROM seven_signs WHERE seven_signs.char_obj_id NOT IN (SELECT obj_Id FROM characters)",
		stmts: []string{
			"exec DELETE FROM bookmarks WHERE bookmarks.obj_Id NOT IN (SELECT obj_Id FROM characters)",
			"exec DELETE FROM buffer_schemes WHERE buffer_schemes.object_id NOT IN (SELECT obj_Id FROM characters)",
			"exec DELETE FROM mods_wedding WHERE mods_wedding.requesterId NOT IN (SELECT obj_Id FROM characters) OR mods_wedding.partnerId NOT IN (SELECT obj_Id FROM characters)",
		},
	},
	{
		after: "exec DELETE FROM character_skills_save WHERE restore_type = 1 AND systime <= ?",
		stmts: []string{"exec " + RetiredIDsSchema},
	},
	{
		after: "query SELECT oid FROM petition",
		stmts: []string{"query SELECT object_id FROM retired_object_ids"},
	},
}

// connector opens connections that record into r.
type connector struct{ r *recorder }

func (c connector) Connect(context.Context) (driver.Conn, error) { return recorderConn{c.r}, nil }
func (c connector) Driver() driver.Driver                        { return nil }

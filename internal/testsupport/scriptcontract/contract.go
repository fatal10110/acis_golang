// Package scriptcontract holds the script engine's contract goldens: tables of
// reference behavior that the engine slices run their implementation against.
//
// Each golden is a named table in testdata. A table names the reference source
// lines it was derived from and its provenance: "probe <revision>" when the
// reference probe produced it by running the reference classes
// (internal/gameserver/script/testdata/oracle/run.sh regenerates those files,
// never edit them), or "hand" when it was derived by reading the cited lines
// and reviewed blind. The file format is described in
// internal/gameserver/script/testdata/oracle/README.md.
//
// A slice that implements a contract runs every row of the matching table:
//
//	scriptcontract.Run(t, "journal.cond_flags", func(t *testing.T, r scriptcontract.Row) {
//		// drive the Go implementation with r's inputs, compare with r's outputs
//	})
package scriptcontract

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"
)

//go:embed testdata/*.golden
var goldens embed.FS

// Source is one reference location a table was derived from.
type Source struct {
	// File is the path below the reference server checkout (aCis_gameserver).
	File     string
	From, To int
	// Symbol names what the lines hold: a method, field or constant.
	Symbol string
}

// Table is one golden.
type Table struct {
	Name string
	// File is the testdata file the table lives in.
	File    string
	Sources []Source
	// Provenance is "hand", or "probe java=<sha> datapack=<sha>".
	Provenance string
	Notes      []string
	Rows       []Row
}

// Hand reports whether the table was derived by hand rather than by the probe.
func (t Table) Hand() bool { return t.Provenance == "hand" }

// Row is one table row: named input and output values, then the ordered
// output lines (packets, statements, hook calls) the row expects.
type Row struct {
	ID     string
	keys   []string
	values map[string]string
	Lines  []string
}

// Keys returns the row's value names in file order.
func (r Row) Keys() []string { return append([]string(nil), r.keys...) }

// Value returns the named value.
func (r Row) Value(key string) (string, bool) {
	v, ok := r.values[key]
	return v, ok
}

// Str returns the named value, failing the test when the row has none.
func (r Row) Str(t testing.TB, key string) string {
	t.Helper()
	v, ok := r.values[key]
	if !ok {
		t.Fatalf("golden row %s has no value %q", r.ID, key)
	}
	return v
}

// Int returns the named value as an integer; hexadecimal values (0x...) are
// read as unsigned 32-bit patterns.
func (r Row) Int(t testing.TB, key string) int64 {
	t.Helper()
	v := r.Str(t, key)
	if rest, ok := strings.CutPrefix(v, "0x"); ok {
		n, err := strconv.ParseUint(rest, 16, 32)
		if err != nil {
			t.Fatalf("golden row %s value %s=%q: %v", r.ID, key, v, err)
		}
		return int64(n)
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		t.Fatalf("golden row %s value %s=%q: %v", r.ID, key, v, err)
	}
	return n
}

// Float returns the named value as a float.
func (r Row) Float(t testing.TB, key string) float64 {
	t.Helper()
	v := r.Str(t, key)
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		t.Fatalf("golden row %s value %s=%q: %v", r.ID, key, v, err)
	}
	return f
}

// Bool returns the named value as a boolean.
func (r Row) Bool(t testing.TB, key string) bool {
	t.Helper()
	v := r.Str(t, key)
	b, err := strconv.ParseBool(v)
	if err != nil {
		t.Fatalf("golden row %s value %s=%q: %v", r.ID, key, v, err)
	}
	return b
}

// List returns the named comma-separated value; "-" is the empty list.
func (r Row) List(t testing.TB, key string) []string {
	t.Helper()
	v := r.Str(t, key)
	if v == "-" {
		return nil
	}
	return strings.Split(v, ",")
}

var (
	loadOnce sync.Once
	loaded   []Table
	loadErr  error
)

// Tables returns every golden, in file and table order.
func Tables() ([]Table, error) {
	loadOnce.Do(func() { loaded, loadErr = load(goldens) })
	return loaded, loadErr
}

// Lookup returns the named golden, failing the test when it is missing.
func Lookup(t testing.TB, name string) Table {
	t.Helper()
	tables, err := Tables()
	if err != nil {
		t.Fatalf("load contract goldens: %v", err)
	}
	for _, tb := range tables {
		if tb.Name == name {
			return tb
		}
	}
	t.Fatalf("no contract golden %q", name)
	return Table{}
}

// Run runs fn as one subtest per row of the named golden.
func Run(t *testing.T, name string, fn func(t *testing.T, r Row)) {
	t.Helper()
	for _, r := range Lookup(t, name).Rows {
		t.Run(r.ID, func(t *testing.T) { fn(t, r) })
	}
}

func load(fsys fs.FS) ([]Table, error) {
	files, err := fs.Glob(fsys, "testdata/*.golden")
	if err != nil {
		return nil, err
	}
	var tables []Table
	for _, f := range files {
		data, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		got, err := parse(path.Base(f), data)
		if err != nil {
			return nil, err
		}
		tables = append(tables, got...)
	}
	return tables, nil
}

// parse reads one golden file. Lines: "# ..." comments; "table <name>" starts
// a table; "source <file> <from>[-<to>] <symbol...>", "provenance <...>" and
// "note <text>" describe it; "row <id> <key>=<value>..." adds a row; lines
// indented by two spaces are the last row's output lines.
func parse(file string, data []byte) ([]Table, error) {
	var tables []Table
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		fail := func(format string, args ...any) error {
			return fmt.Errorf("%s:%d: %s", file, n, fmt.Sprintf(format, args...))
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "  "); ok {
			if len(tables) == 0 || len(tables[len(tables)-1].Rows) == 0 {
				return nil, fail("output line outside a row")
			}
			rows := tables[len(tables)-1].Rows
			rows[len(rows)-1].Lines = append(rows[len(rows)-1].Lines, rest)
			continue
		}
		word, rest, _ := strings.Cut(line, " ")
		if word == "table" {
			tables = append(tables, Table{Name: rest, File: file})
			continue
		}
		if len(tables) == 0 {
			return nil, fail("%s before the first table", word)
		}
		tb := &tables[len(tables)-1]
		switch word {
		case "source":
			src, err := parseSource(rest)
			if err != nil {
				return nil, fail("%v", err)
			}
			tb.Sources = append(tb.Sources, src)
		case "provenance":
			tb.Provenance = rest
		case "note":
			tb.Notes = append(tb.Notes, rest)
		case "row":
			r, err := parseRow(rest)
			if err != nil {
				return nil, fail("%v", err)
			}
			tb.Rows = append(tb.Rows, r)
		default:
			return nil, fail("unknown line %q", word)
		}
	}
	return tables, sc.Err()
}

func parseSource(s string) (Source, error) {
	fields := strings.SplitN(s, " ", 3)
	if len(fields) < 3 {
		return Source{}, fmt.Errorf("source %q: want <file> <lines> <symbol>", s)
	}
	from, to, ranged := strings.Cut(fields[1], "-")
	if !ranged {
		to = from
	}
	a, err := strconv.Atoi(from)
	if err != nil {
		return Source{}, fmt.Errorf("source %q: %w", s, err)
	}
	b, err := strconv.Atoi(to)
	if err != nil {
		return Source{}, fmt.Errorf("source %q: %w", s, err)
	}
	return Source{File: fields[0], From: a, To: b, Symbol: fields[2]}, nil
}

func parseRow(s string) (Row, error) {
	id, rest, _ := strings.Cut(s, " ")
	r := Row{ID: id, values: map[string]string{}}
	for rest != "" {
		key, after, ok := strings.Cut(rest, "=")
		if !ok || key == "" || strings.Contains(key, " ") {
			return Row{}, fmt.Errorf("row %s: malformed value at %q", id, rest)
		}
		var value string
		if strings.HasPrefix(after, `"`) {
			end := quotedEnd(after)
			if end < 0 {
				return Row{}, fmt.Errorf("row %s: unterminated quote in %s", id, key)
			}
			v, err := strconv.Unquote(after[:end])
			if err != nil {
				return Row{}, fmt.Errorf("row %s value %s: %w", id, key, err)
			}
			value, after = v, after[end:]
		} else {
			value, after, _ = strings.Cut(after, " ")
			after = " " + after
		}
		if _, dup := r.values[key]; dup {
			return Row{}, fmt.Errorf("row %s: duplicate value %s", id, key)
		}
		r.keys = append(r.keys, key)
		r.values[key] = value
		rest = strings.TrimPrefix(after, " ")
	}
	return r, nil
}

// quotedEnd returns the index just past the closing quote of the quoted string
// at the start of s, or -1.
func quotedEnd(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i + 1
		}
	}
	return -1
}

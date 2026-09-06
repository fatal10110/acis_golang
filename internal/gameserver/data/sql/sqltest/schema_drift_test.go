package sqltest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSchemaMatchesDatapack(t *testing.T) {
	dir := datapackSQLDir()
	if dir == "" {
		t.Skip("datapack sql directory not present; skipping schema drift check")
	}

	// Drive the check from schemaStmts itself (sqltest.go), rather than a
	// separate name-to-constant map, so a constant added there without a
	// matching entry here can't go unchecked. sevenSignsStatusSeed is an
	// INSERT, not a CREATE TABLE, and is skipped below.
	for _, stmt := range schemaStmts {
		copySchema, ok := createTableStatement(stmt)
		if !ok {
			continue
		}
		table, ok := tableName(copySchema)
		if !ok {
			t.Fatalf("no table name in CREATE TABLE statement: %s", copySchema)
		}

		t.Run(table, func(t *testing.T) {
			file := table + ".sql"
			data, err := os.ReadFile(filepath.Join(dir, file))
			if err != nil {
				t.Fatalf("read shipped schema: %v", err)
			}
			shipped, ok := createTableStatement(string(data))
			if !ok {
				t.Fatalf("no CREATE TABLE statement in %s", file)
			}
			if copySchema != shipped {
				t.Fatalf("schema drifted from %s\n copy    = %s\n shipped = %s", file, copySchema, shipped)
			}
		})
	}
}

// datapackSQLDir locates aCis_datapack/sql as a sibling of the outer
// workspace root, walking up from the working directory to find it. The
// primary checkout and every linked worktree sit at different depths under
// that root, so this checks each ancestor rather than assuming one fixed
// relative path. It returns "" when the datapack is not checked out (CI has
// no copy of it).
func datapackSQLDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		sql := filepath.Join(dir, "aCis_datapack", "sql")
		if info, err := os.Stat(sql); err == nil && info.IsDir() {
			return sql
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

var (
	sqlLineComment  = regexp.MustCompile(`(?m)(--[ \t][^\n]*|#[^\n]*)`)
	sqlBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	sqlWhitespace   = regexp.MustCompile(`\s+`)
	sqlIfNotExists  = regexp.MustCompile(`(?i)\bIF NOT EXISTS\b`)
	sqlTableName    = regexp.MustCompile(`(?i)^CREATE TABLE\s+(\S+)\s*\(`)
)

// createTableStatement reduces a .sql file, or one of the schemaStmts
// constants, to its single CREATE TABLE statement in a form that ignores
// differences the copies deliberately carry: comments, indentation and line
// breaks, backtick quoting, a statement terminator (including a trailing
// INSERT such as seven_signs_status ships after its CREATE TABLE), and the
// IF NOT EXISTS clause the test copies add so repeated setup is idempotent.
// It does not ignore table options (ENGINE, CHARSET, ...) — none of the
// shipped tables carry any today, and silently dropping them would let a
// real divergence there pass unnoticed.
func createTableStatement(text string) (string, bool) {
	text = sqlBlockComment.ReplaceAllString(text, " ")
	text = sqlLineComment.ReplaceAllString(text, " ")
	text = strings.ReplaceAll(text, "`", "")

	start := strings.Index(strings.ToUpper(text), "CREATE TABLE")
	if start < 0 {
		return "", false
	}
	text = text[start:]
	if semi := strings.Index(text, ";"); semi >= 0 {
		text = text[:semi]
	}

	text = sqlIfNotExists.ReplaceAllString(text, " ")
	return strings.TrimSpace(sqlWhitespace.ReplaceAllString(text, " ")), true
}

// tableName extracts the identifier from a createTableStatement result
// (e.g. "characters" from "CREATE TABLE characters ( ... )").
func tableName(statement string) (string, bool) {
	m := sqlTableName.FindStringSubmatch(statement)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// TestCreateTableStatementDetectsTableOptionDrift guards the review finding
// on PR #2293 (fatal10110/acis_golang): an earlier version of
// createTableStatement truncated at the last ")" in the statement, which
// discarded any trailing table options (ENGINE, CHARSET, ...) along with
// the intended terminator. Two schemas differing only in a table option
// then normalized to the same string and TestSchemaMatchesDatapack could
// not have caught a real drift there. None of the shipped tables carry
// table options today, but the comparison must still see them.
func TestCreateTableStatementDetectsTableOptionDrift(t *testing.T) {
	base, ok := createTableStatement("CREATE TABLE `x` (`a` INT);")
	if !ok {
		t.Fatal("createTableStatement: no CREATE TABLE found")
	}
	withEngine, ok := createTableStatement("CREATE TABLE `x` (`a` INT) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;")
	if !ok {
		t.Fatal("createTableStatement: no CREATE TABLE found")
	}
	if base == withEngine {
		t.Fatalf("createTableStatement ignored table options: both normalized to %q", base)
	}
}

func TestTableNameExtractsFromCreateTable(t *testing.T) {
	stmt, ok := createTableStatement("CREATE TABLE IF NOT EXISTS `character_hennas` (\n `char_obj_id` INT\n);")
	if !ok {
		t.Fatal("createTableStatement: no CREATE TABLE found")
	}
	name, ok := tableName(stmt)
	if !ok {
		t.Fatalf("tableName: no match in %q", stmt)
	}
	if name != "character_hennas" {
		t.Fatalf("tableName = %q, want character_hennas", name)
	}
}

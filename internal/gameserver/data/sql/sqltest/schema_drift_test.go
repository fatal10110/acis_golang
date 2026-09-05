package sqltest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// datapackSchemas maps each hand-copied CREATE TABLE constant above to the
// shipped .sql file it was copied from, so a datapack schema change that
// this package never picked up fails here instead of silently letting
// integration tests run against a stale table definition.
var datapackSchemas = map[string]string{
	"characters.sql":            charactersSchema,
	"items.sql":                 itemsSchema,
	"augmentations.sql":         augmentationsSchema,
	"spawn_data.sql":            spawnDataSchema,
	"items_on_ground.sql":       itemsOnGroundSchema,
	"character_skills.sql":      characterSkillsSchema,
	"character_shortcuts.sql":   characterShortcutsSchema,
	"character_hennas.sql":      characterHennasSchema,
	"pets.sql":                  petsSchema,
	"character_skills_save.sql": characterSkillsSaveSchema,
	"seven_signs_status.sql":    sevenSignsStatusSchema,
}

func TestSchemaMatchesDatapack(t *testing.T) {
	dir := datapackSQLDir()
	if dir == "" {
		t.Skip("datapack sql directory not present; skipping schema drift check")
	}

	for name, schema := range datapackSchemas {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read shipped schema: %v", err)
			}
			shipped, ok := createTableStatement(string(data))
			if !ok {
				t.Fatalf("no CREATE TABLE statement in %s", name)
			}
			got, ok := createTableStatement(schema)
			if !ok {
				t.Fatalf("no CREATE TABLE statement in the copy of %s", name)
			}
			if got != shipped {
				t.Fatalf("schema drifted from %s\n copy    = %s\n shipped = %s", name, got, shipped)
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
)

// createTableStatement reduces a .sql file, or one of the constants above,
// to its single CREATE TABLE statement in a form that ignores differences
// the copies deliberately carry: comments, indentation and line breaks,
// backtick quoting, a statement terminator, and the IF NOT EXISTS clause
// the test copies add so repeated setup is idempotent.
func createTableStatement(text string) (string, bool) {
	text = sqlBlockComment.ReplaceAllString(text, " ")
	text = sqlLineComment.ReplaceAllString(text, " ")
	text = strings.ReplaceAll(text, "`", "")

	start := strings.Index(strings.ToUpper(text), "CREATE TABLE")
	if start < 0 {
		return "", false
	}
	text = text[start:]
	// Stop at the statement terminator so a trailing INSERT (seven_signs_status
	// ships a seed row after its CREATE TABLE) isn't folded into the compare.
	if semi := strings.Index(text, ";"); semi >= 0 {
		text = text[:semi]
	}
	if end := strings.LastIndex(text, ")"); end >= 0 {
		text = text[:end+1]
	}

	text = sqlIfNotExists.ReplaceAllString(text, " ")
	return strings.TrimSpace(sqlWhitespace.ReplaceAllString(text, " ")), true
}

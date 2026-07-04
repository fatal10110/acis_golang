package xml

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// datapackPath resolves a path under the shared aCis_datapack checkout
// relative to this test file, so the test works regardless of the working
// directory `go test` is invoked from.
func datapackPath(t *testing.T, rel string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed to resolve test file path")
	}
	// this file lives at <workspace>/<worktree>/internal/gameserver/data/xml
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..", "aCis_datapack", rel)
}

func TestLoadPlayerLevelTable(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "playerLevels.xml"))

	table, err := LoadPlayerLevelTable(path)
	if err != nil {
		t.Fatalf("LoadPlayerLevelTable(%q) error: %v", path, err)
	}

	const wantCount = 81
	if got := table.Count(); got != wantCount {
		t.Fatalf("Count() = %d, want %d", got, wantCount)
	}

	const wantMaxLevel = 81
	if got := table.MaxLevel(); got != wantMaxLevel {
		t.Fatalf("MaxLevel() = %d, want %d", got, wantMaxLevel)
	}
	if got := table.RealMaxLevel(); got != wantMaxLevel-1 {
		t.Fatalf("RealMaxLevel() = %d, want %d", got, wantMaxLevel-1)
	}

	if exp, ok := table.RequiredExpForHighestLevel(); !ok || exp != 6299994999 {
		t.Fatalf("RequiredExpForHighestLevel() = (%d, %v), want (6299994999, true)", exp, ok)
	}

	cases := []struct {
		name  string
		level int
		want  PlayerLevel
	}{
		{
			name:  "level 1 (boundary, requires no exp)",
			level: 1,
			want: PlayerLevel{
				RequiredExp:    0,
				KarmaModifier:  0.772184315,
				ExpLossAtDeath: 10.0,
			},
		},
		{
			name:  "level 50 (mid-range)",
			level: 50,
			want: PlayerLevel{
				RequiredExp:    40153995,
				KarmaModifier:  17.18356182,
				ExpLossAtDeath: 4.0,
			},
		},
		{
			name:  "level 80 (highest attainable level)",
			level: 80,
			want: PlayerLevel{
				RequiredExp:    4200000000,
				KarmaModifier:  29.77769028,
				ExpLossAtDeath: 1.0,
			},
		},
		{
			name:  "level 81 (sentinel, missing optional attributes default to 0)",
			level: 81,
			want: PlayerLevel{
				RequiredExp:    6299994999,
				KarmaModifier:  0,
				ExpLossAtDeath: 0,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := table.Level(c.level)
			if !ok {
				t.Fatalf("Level(%d) missing, want present", c.level)
			}
			if got != c.want {
				t.Errorf("Level(%d) = %+v, want %+v", c.level, got, c.want)
			}
		})
	}

	if _, ok := table.Level(0); ok {
		t.Errorf("Level(0) present, want absent (no such level in the table)")
	}
	if _, ok := table.Level(82); ok {
		t.Errorf("Level(82) present, want absent (beyond the sentinel max level)")
	}
}

func TestLoadPlayerLevelTableErrors(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing file", func(t *testing.T) {
		if _, err := LoadPlayerLevelTable(filepath.Join(dir, "does-not-exist.xml")); err == nil {
			t.Fatal("expected an error for a missing file, got nil")
		}
	})

	t.Run("malformed xml", func(t *testing.T) {
		path := filepath.Join(dir, "malformed.xml")
		writeFile(t, path, `<list><playerLevel level="1" requiredExpToLevelUp="0" </list>`)
		if _, err := LoadPlayerLevelTable(path); err == nil {
			t.Fatal("expected an error for malformed XML, got nil")
		}
	})

	t.Run("missing required level attribute", func(t *testing.T) {
		path := filepath.Join(dir, "missing-level.xml")
		writeFile(t, path, `<list><playerLevel requiredExpToLevelUp="0" /></list>`)
		if _, err := LoadPlayerLevelTable(path); err == nil {
			t.Fatal("expected an error for a missing level attribute, got nil")
		}
	})

	t.Run("missing required requiredExpToLevelUp attribute", func(t *testing.T) {
		path := filepath.Join(dir, "missing-exp.xml")
		writeFile(t, path, `<list><playerLevel level="1" /></list>`)
		if _, err := LoadPlayerLevelTable(path); err == nil {
			t.Fatal("expected an error for a missing requiredExpToLevelUp attribute, got nil")
		}
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test fixture %q: %v", path, err)
	}
}

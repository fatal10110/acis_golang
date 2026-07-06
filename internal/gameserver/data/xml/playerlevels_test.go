package xml

import (
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

func TestLoadPlayerLevels(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "playerLevels.xml"))

	table, err := LoadPlayerLevels(path)
	if err != nil {
		t.Fatalf("LoadPlayerLevels(%q) error: %v", path, err)
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

	if exp := table.RequiredExpForHighestLevel(); exp != 6299994999 {
		t.Fatalf("RequiredExpForHighestLevel() = %d, want 6299994999", exp)
	}

	cases := []struct {
		name  string
		level int
		want  player.Level
	}{
		{
			name:  "level 1 (boundary, requires no exp)",
			level: 1,
			want: player.Level{
				RequiredExpToLevelUp: 0,
				KarmaModifier:        0.772184315,
				ExpLossAtDeath:       10.0,
			},
		},
		{
			name:  "level 50 (mid-range)",
			level: 50,
			want: player.Level{
				RequiredExpToLevelUp: 40153995,
				KarmaModifier:        17.18356182,
				ExpLossAtDeath:       4.0,
			},
		},
		{
			name:  "level 80 (highest attainable level)",
			level: 80,
			want: player.Level{
				RequiredExpToLevelUp: 4200000000,
				KarmaModifier:        29.77769028,
				ExpLossAtDeath:       1.0,
			},
		},
		{
			name:  "level 81 (sentinel, missing optional attributes default to 0)",
			level: 81,
			want: player.Level{
				RequiredExpToLevelUp: 6299994999,
				KarmaModifier:        0,
				ExpLossAtDeath:       0,
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

func TestLoadPlayerLevelsErrors(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "malformed xml",
			content: `<list><playerLevel level="1" requiredExpToLevelUp="0" </list>`,
		},
		{
			name:    "missing required level attribute",
			content: `<list><playerLevel requiredExpToLevelUp="0" /></list>`,
		},
		{
			name:    "missing required requiredExpToLevelUp attribute",
			content: `<list><playerLevel level="1" /></list>`,
		},
		{
			name:    "malformed optional karmaModifier attribute",
			content: `<list><playerLevel level="1" requiredExpToLevelUp="0" karmaModifier="oops" /></list>`,
		},
		{
			name:    "empty table",
			content: `<list></list>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, "fixture.xml")
			writeXMLFixture(t, path, c.content)
			if _, err := LoadPlayerLevels(path); err == nil {
				t.Fatalf("expected an error for %s, got nil", c.name)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		if _, err := LoadPlayerLevels(filepath.Join(dir, "does-not-exist.xml")); err == nil {
			t.Fatal("expected an error for a missing file, got nil")
		}
	})
}

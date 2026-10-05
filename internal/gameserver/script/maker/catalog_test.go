package maker

import (
	"path/filepath"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// TestCatalogAgainstShippedSpawnlist pins the maker each shipped npcmaker
// runs: the 13 no_on_start_maker and 231 event_maker npcmakers their own
// makers, and the 7,605 default_maker npcmakers and the 958 permanent
// default ones (random_spawn_treasurebox, random_maker) the default maker.
func TestCatalogAgainstShippedSpawnlist(t *testing.T) {
	dp := datapack.Require(t)
	table, err := gamexml.LoadSpawnlist(filepath.Join(dp, "data", "xml", "spawnlist"), zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	makers := script.NewMakers(Catalog(), Default, zerolog.Nop())
	byType := map[string]int{}
	for _, m := range table.Makers() {
		byType[m.AIType]++
	}
	for _, tc := range []struct {
		types      []string
		count      int
		registered bool
	}{
		{[]string{"no_on_start_maker"}, 13, true},
		{[]string{"event_maker"}, 231, true},
		{[]string{"default_maker"}, 7605, false},
		{[]string{"random_spawn_treasurebox", "random_maker"}, 958, false},
	} {
		count := 0
		for _, typ := range tc.types {
			count += byType[typ]
			if got := makers.Registered(typ); got != tc.registered {
				t.Errorf("%s registered = %v, want %v", typ, got, tc.registered)
			}
		}
		if count != tc.count {
			t.Errorf("npcmakers of %v = %d, want %d", tc.types, count, tc.count)
		}
	}
	if len(Catalog()) != 2 {
		t.Errorf("catalog types = %d, want 2", len(Catalog()))
	}
}

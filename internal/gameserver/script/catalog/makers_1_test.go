package catalog

import (
	"path/filepath"
	"reflect"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// TestMakers1Bindings pins the maker each ported batch 1 type binds, by
// constructor, and how many shipped npcmakers run it: 4 Close_Door_maker,
// 28 default_use_db_maker, 2 farm_maker and 53 maker_instant_spawn.
func TestMakers1Bindings(t *testing.T) {
	dp := datapack.Require(t)
	table, err := gamexml.LoadSpawnlist(filepath.Join(dp, "data", "xml", "spawnlist"), zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	byType := map[string]int{}
	for _, m := range table.Makers() {
		byType[m.AIType]++
	}
	makers := makers1()
	ported := 0
	for _, ctor := range makers {
		if ctor != nil {
			ported++
		}
	}
	for _, tc := range []struct {
		typ   string
		ctor  func() script.Maker
		count int
	}{
		{"Close_Door_maker", maker.CloseDoor, 4},
		{"default_use_db_maker", maker.DefaultUseDB, 28},
		{"farm_maker", maker.Farm, 2},
		{"maker_instant_spawn", maker.InstantSpawn, 53},
	} {
		got := makers[tc.typ]
		if got == nil || reflect.ValueOf(got).Pointer() != reflect.ValueOf(tc.ctor).Pointer() {
			t.Errorf("%s does not run its own maker", tc.typ)
		}
		if n := byType[tc.typ]; n != tc.count {
			t.Errorf("shipped npcmakers of %s = %d, want %d", tc.typ, n, tc.count)
		}
	}
	if ported != 4 {
		t.Errorf("ported batch 1 types = %d, want 4", ported)
	}
}

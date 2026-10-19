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

// TestMakers3Bindings pins the maker each ported batch 3 type binds, by
// constructor, and how many shipped npcmakers run it: two
// frintessa_evilate_maker npcmakers and one of each other type.
func TestMakers3Bindings(t *testing.T) {
	dp := datapack.Require(t)
	table, err := gamexml.LoadSpawnlist(filepath.Join(dp, "data", "xml", "spawnlist"), zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	byType := map[string]int{}
	for _, m := range table.Makers() {
		byType[m.AIType]++
	}
	makers := makers3()
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
		{"benom_maker", maker.Benom, 1},
		{"freya_deacon_keeper_maker", maker.FreyaDeaconKeeper, 1},
		{"frintessa_evilate_maker", maker.FrintezzaEvilate, 2},
		{"sailren_dummy_maker", maker.SailrenDummy, 1},
		{"sculpture_garden_maker", maker.SculptureGarden, 1},
		{"sculpture_ice_fairy_maker", maker.SculptureIceFairy, 1},
		{"statue_of_shilen_maker", maker.StatueOfShilen, 1},
		{"velociraptor_maker", maker.Velociraptor, 1},
	} {
		got := makers[tc.typ]
		if got == nil || reflect.ValueOf(got).Pointer() != reflect.ValueOf(tc.ctor).Pointer() {
			t.Errorf("%s does not run its own maker", tc.typ)
		}
		if n := byType[tc.typ]; n != tc.count {
			t.Errorf("shipped npcmakers of %s = %d, want %d", tc.typ, n, tc.count)
		}
	}
	if ported != 8 {
		t.Errorf("ported batch 3 types = %d, want 8", ported)
	}
}

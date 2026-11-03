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

// TestMakers1Bindings pins the maker every batch 1 type binds, by
// constructor, and how many shipped npcmakers run it: 4 Close_Door_maker,
// 28 default_use_db_maker, 5 exclusive_despawn_normal, 12
// exclusive_spawn_normal and 2 exclusive_spawn_use_db (both
// ExclusiveSpawnNormal), 2 farm_maker, 53 maker_instant_spawn, 439
// maker_instant_spawn_random, 3 maker_instant_spawn_serial, 3
// maker_instant_spawn_serial_last, 98 on_day_night_spawn, 1
// parent_spawn_all, 372 random_spawn, 13 random_spawn_pawn, 26
// unique_npc_kill_event and the 7 undead_band door makers (1 lance, 4
// leader, 2 archer_wazird).
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
		{"exclusive_despawn_normal", maker.ExclusiveDespawnNormal, 5},
		{"exclusive_spawn_normal", maker.ExclusiveSpawnNormal, 12},
		{"exclusive_spawn_use_db", maker.ExclusiveSpawnNormal, 2},
		{"farm_maker", maker.Farm, 2},
		{"maker_instant_spawn", maker.InstantSpawn, 53},
		{"maker_instant_spawn_random", maker.InstantSpawnRandom, 439},
		{"maker_instant_spawn_serial", maker.InstantSpawnSerial, 3},
		{"maker_instant_spawn_serial_last", maker.InstantSpawnSerialLast, 3},
		{"on_day_night_spawn", maker.OnDayNightSpawn, 98},
		{"parent_spawn_all", maker.ParentSpawnAll, 1},
		{"random_spawn", maker.RandomSpawn, 372},
		{"random_spawn_pawn", maker.RandomSpawnPawn, 13},
		{"unique_npc_kill_event", maker.UniqueNpcKillEvent, 26},
		{"undead_band_lance_maker", maker.OpenDoor, 1},
		{"undead_band_leader_maker", maker.OpenDoor, 4},
		{"undead_band_archer_wazird_maker", maker.OpenDoor, 2},
	} {
		got := makers[tc.typ]
		if got == nil || reflect.ValueOf(got).Pointer() != reflect.ValueOf(tc.ctor).Pointer() {
			t.Errorf("%s does not run its own maker", tc.typ)
		}
		if n := byType[tc.typ]; n != tc.count {
			t.Errorf("shipped npcmakers of %s = %d, want %d", tc.typ, n, tc.count)
		}
	}
	if ported != 18 || len(makers) != 18 {
		t.Errorf("ported batch 1 types = %d of %d, want all 18", ported, len(makers))
	}
}

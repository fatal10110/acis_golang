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

// TestMakers2Bindings pins the maker each ported batch 2 type binds, by
// constructor, and how many shipped npcmakers run it: one npcmaker of each
// alarm device type, the 10 hall keeper and hall timer bomb npcmakers (5
// wizard, 1 each of the others), the one rainbow_maker, the one
// npcmaker of each Imperial Tomb hall maker, the 54 dimensional rift rooms
// and the 112 Royal Rush npcmakers of the Four Sepulchers.
func TestMakers2Bindings(t *testing.T) {
	dp := datapack.Require(t)
	table, err := gamexml.LoadSpawnlist(filepath.Join(dp, "data", "xml", "spawnlist"), zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	byType := map[string]int{}
	for _, m := range table.Makers() {
		byType[m.AIType]++
	}
	makers := makers2()
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
		{"alarm_device1_maker", maker.AlarmDevice1, 1},
		{"alarm_device2_maker", maker.AlarmDevice2, 1},
		{"alarm_device3_maker", maker.AlarmDevice3, 1},
		{"alarm_device4_maker", maker.AlarmDevice4, 1},
		{"hall_keeper_wizard_maker", maker.HallKeeperDefault, 5},
		{"hall_keeper_patrol_maker", maker.HallKeeperDefault, 1},
		{"hall_keeper_guard_maker", maker.HallKeeperDefault, 1},
		{"hall_keeper_captain_maker", maker.HallKeeperDefault, 1},
		{"hall_keeper_self_destruction_maker", maker.HallKeeperDefault, 1},
		{"hall_timer_bomb_maker", maker.HallKeeperDefault, 1},
		{"rainbow_maker", maker.Rainbow, 1},
		{"undead_bandmaster_maker", maker.UndeadBandmaster, 1},
		{"warrior_passive_weakness_maker", maker.WarriorPassiveWeakness, 1},
		{"manage_teleport_dungeon", maker.ManageTeleportDungeon, 54},
		{"royal_rush_maker", maker.RoyalRush, 68},
		{"royal_req_next_maker", maker.RoyalReqNext, 40},
		{"royal_spawn_treasurebox", maker.RoyalSpawnTreasureBox, 4},
	} {
		got := makers[tc.typ]
		if got == nil || reflect.ValueOf(got).Pointer() != reflect.ValueOf(tc.ctor).Pointer() {
			t.Errorf("%s does not run its own maker", tc.typ)
		}
		if n := byType[tc.typ]; n != tc.count {
			t.Errorf("shipped npcmakers of %s = %d, want %d", tc.typ, n, tc.count)
		}
	}
	if ported != 17 {
		t.Errorf("ported batch 2 types = %d, want 17", ported)
	}
}

package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// TestFolkSpawnTerritoryContains (MultiSpawn.isInMyTerritory,
// Spawn.isInMyTerritory): a civilian NPC of an npcmaker counts its maker's
// territory less the banned one; one of a plain spawn a strict sphere of
// the drift range around its spawn point; one with no spawn point
// anywhere.
func TestFolkSpawnTerritoryContains(t *testing.T) {
	folk := func(t *testing.T, inst func(*Instance)) *Folk {
		t.Helper()
		in, err := NewInstance(1, &Template{ID: 32104, TemplateID: 32104, Type: "Folk", Level: 70, HPMax: 2444})
		if err != nil {
			t.Fatal(err)
		}
		inst(in)
		f, err := NewFolk(in)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	square := func(t *testing.T, name string, minZ, maxZ, x0, y0, x1, y1 int) *spawn.Territory {
		t.Helper()
		tr, err := spawn.NewTerritory(name, minZ, maxZ, []spawn.Node{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}})
		if err != nil {
			t.Fatal(err)
		}
		return tr
	}

	maker := folk(t, func(in *Instance) {
		in.Maker = &spawn.Maker{
			Territories:       []*spawn.Territory{square(t, "field", -100, 100, 0, 0, 1000, 1000)},
			BannedTerritories: []*spawn.Territory{square(t, "ban", -100, 100, 0, 0, 100, 100)},
		}
		in.Home, in.HasHome = location.Location{X: 500, Y: 500}, true
	})
	plain := folk(t, func(in *Instance) { in.Home, in.HasHome = location.Location{X: 500, Y: 500}, true })
	homeless := folk(t, func(*Instance) {})

	for _, tc := range []struct {
		name string
		f    *Folk
		at   location.Location
		want bool
	}{
		{"maker: in its territory", maker, location.Location{X: 900, Y: 900}, true},
		{"maker: in the banned part", maker, location.Location{X: 50, Y: 50}, false},
		{"maker: outside", maker, location.Location{X: 1500, Y: 500}, false},
		{"maker: above the territory", maker, location.Location{X: 900, Y: 900, Z: 500}, false},
		{"plain: within the drift range", plain, location.Location{X: 699, Y: 500}, true},
		{"plain: at the drift range", plain, location.Location{X: 700, Y: 500}, false},
		{"no spawn point", homeless, location.Location{X: 90000, Y: 90000}, true},
	} {
		if got := tc.f.SpawnTerritoryContains(tc.at); got != tc.want {
			t.Errorf("%s: SpawnTerritoryContains(%+v) = %v, want %v", tc.name, tc.at, got, tc.want)
		}
	}
}

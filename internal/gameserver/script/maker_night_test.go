package script

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/rs/zerolog"
)

// TestMakerReadsTheTimeOfDay pins GameTimeTaskManager.isNight as a maker
// reads it: each maker the registry builds asks the registry's clock at
// the moment of the read.
func TestMakerReadsTheTimeOfDay(t *testing.T) {
	night := false
	var seen []bool
	ctor := func() Maker {
		return Maker{MakerHooks: MakerHooks{OnStart: func(m *Maker, _ MakerStart) {
			seen = append(seen, m.IsNight())
		}}}
	}
	r := NewMakers(MakerCatalog{"on_day_night_spawn": ctor}, ctor, MakerWorld{Night: func() bool { return night }}, zerolog.Nop())
	g := fakeGroup{def: &spawn.Maker{Name: "m"}}
	run := r.New("on_day_night_spawn")
	run.Start(g)
	night = true
	run.Start(g)
	r.New("default_maker").Start(g)
	if want := []bool{false, true, true}; !slices.Equal(seen, want) {
		t.Fatalf("IsNight reads %v, want %v", seen, want)
	}
}

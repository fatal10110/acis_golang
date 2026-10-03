package npc

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// newWorldFolk builds a merchant attached to w on an inline queue and
// places it at (100, 0, 0).
func newWorldFolk(t *testing.T, w *world.State) (*Folk, *sim.Inline) {
	t.Helper()
	inst, err := NewInstance(1, &Template{ID: 30001, TemplateID: 30001, Type: "Merchant", Level: 70, HPMax: 2444})
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFolk(inst)
	if err != nil {
		t.Fatal(err)
	}
	in := sim.NewInline(time.Unix(0, 0))
	if err := f.Attach(FolkRuntime{World: w, Queue: in.NewQueue("folk")}); err != nil {
		t.Fatal(err)
	}
	w.Spawn(f, 100, 0, 0, 0)
	return f, in
}

// TestFolkAttachRefusesNilQueue pins that a civilian NPC cannot be attached
// without the queue its regeneration and stance expiry post to.
func TestFolkAttachRefusesNilQueue(t *testing.T) {
	inst, err := NewInstance(1, &Template{ID: 30001, TemplateID: 30001, Type: "Merchant", Level: 70, HPMax: 2444})
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFolk(inst)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Attach(FolkRuntime{World: world.New()}); err == nil {
		t.Fatal("Attach without a queue = nil error, want a refusal")
	}
}

// TestFolkInactiveRegionStopsEffects follows Npc.onInactiveRegion
// (stopAllEffects): a debuffed civilian NPC whose region loses its last
// player drops its effects, unless a player wakes the region again before
// the stop posted to the NPC's queue runs.
func TestFolkInactiveRegionStopsEffects(t *testing.T) {
	for _, tc := range []struct {
		name       string
		reactivate bool
		wantHeld   int
	}{
		{"region stays inactive", false, 0},
		{"region reactivated first", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := world.New()
			f, in := newWorldFolk(t, w)
			visitor := &hostileTarget{id: 2, playable: true}
			w.Spawn(visitor, 100, 0, 0, 0)
			if _, active := w.RegionActivity(f); !active {
				t.Fatal("merchant's region inactive with a player in it")
			}

			debuff, err := effect.New(effect.Skill{ID: 1, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Debuff", Time: 60})
			if err != nil {
				t.Fatal(err)
			}
			debuff.Effector, debuff.Effected = visitor, f
			f.Queue().Post(func() { f.EffectList().Add(debuff) })
			in.Run()
			if got := len(f.EffectList().All()); got != 1 {
				t.Fatalf("merchant holds %d effects after the debuff, want 1", got)
			}

			w.Despawn(visitor)
			if _, active := w.RegionActivity(f); active {
				t.Fatal("merchant's region still active after its last player left")
			}
			if tc.reactivate {
				w.Spawn(&hostileTarget{id: 3, playable: true}, 100, 0, 0, 0)
			}
			in.Run()

			if got := len(f.EffectList().All()); got != tc.wantHeld {
				t.Fatalf("merchant holds %d effects after the region stop ran, want %d", got, tc.wantHeld)
			}
		})
	}
}

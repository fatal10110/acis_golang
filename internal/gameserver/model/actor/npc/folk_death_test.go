package npc

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// nopFolkDecay is a decay effect that removes nothing: the tests only read
// the registrations.
type nopFolkDecay struct{}

func (nopFolkDecay) Decay(task.DecayActor) {}

// newMortalFolk builds a mortal walking merchant with a recorded sink and,
// when decay is non-nil, a corpse-decay task.
func newMortalFolk(t *testing.T, objectID int32, decay *task.Decay) (*Folk, *event.Recorder) {
	t.Helper()
	inst, err := NewInstance(objectID, &Template{
		ID: 30001, TemplateID: 30001, Type: "Merchant", Level: 70,
		HPMax: 2444, MPMax: 1000, CorpseTime: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	inst.WalkMode = true
	f, err := NewFolk(inst, false)
	if err != nil {
		t.Fatal(err)
	}
	rec := &event.Recorder{}
	q := sim.NewInline(time.Unix(0, 0)).NewQueue("folk")
	if err := f.Attach(FolkRuntime{Queue: q, Sink: rec, Decay: decay}); err != nil {
		t.Fatal(err)
	}
	return f, rec
}

// newFolkAttacker builds a civilian NPC that may deal damage, to hit with.
func newFolkAttacker(t *testing.T) *Folk {
	t.Helper()
	inst, err := NewInstance(99, &Template{ID: 30002, TemplateID: 30002, Type: "Merchant", Level: 70, HPMax: 100})
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFolk(inst, false)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestDeadFolkRefusesVitalsChanges follows CreatureStatus.reduceHp (a dead
// actor returns first), setHp/setMp (refused on a dead actor) and
// Creature.doDie (stopHpMpRegeneration): once a mortal civilian NPC dies,
// no hit, heal, HP set, MP change or regeneration tick moves its HP or MP,
// switches its stance or offers its health bar again.
func TestDeadFolkRefusesVitalsChanges(t *testing.T) {
	attacker := newFolkAttacker(t)
	for _, tc := range []struct {
		name  string
		apply func(f *Folk)
	}{
		{"hit from an attacker", func(f *Folk) {
			if f.TakeDamage(10, attacker) {
				t.Error("TakeDamage on a corpse reported a kill")
			}
		}},
		{"skill damage", func(f *Folk) { f.ReduceHP(10, attacker, modelskill.Definition{}) }},
		{"kill", func(f *Folk) {
			if f.Kill(attacker) {
				t.Error("Kill on a corpse reported a kill")
			}
		}},
		{"regeneration tick", func(f *Folk) { f.TickRegen() }},
		{"heal", func(f *Folk) {
			if got := f.AddHP(500); got != 0 {
				t.Errorf("AddHP on a corpse applied %v, want 0", got)
			}
		}},
		{"set HP", func(f *Folk) { f.SetHP(500) }},
		{"MP restore", func(f *Folk) {
			if got := f.AddMP(100); got != 0 {
				t.Errorf("AddMP on a corpse applied %v, want 0", got)
			}
		}},
		{"MP drain", func(f *Folk) {
			if got := f.ReduceMP(100); got != 0 {
				t.Errorf("ReduceMP on a corpse removed %v, want 0", got)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, rec := newMortalFolk(t, 1, nil)
			// MP short of its max and above zero, so a restore, a drain
			// and a regeneration tick would each move it.
			if got := f.ReduceMP(500); got != 500 {
				t.Fatalf("ReduceMP before death removed %v, want 500", got)
			}
			if !f.Kill(nil) {
				t.Fatal("Kill on a live mortal merchant reported no kill")
			}
			if f.Running() {
				t.Fatal("walking merchant runs after a Kill with no attacker")
			}
			before := len(rec.Events())

			tc.apply(f)

			if hp := f.HP(); hp != 0 {
				t.Errorf("corpse HP = %v, want 0", hp)
			}
			if mp := f.MPValue(); mp != 500 {
				t.Errorf("corpse MP = %v, want 500", mp)
			}
			if f.Running() {
				t.Error("corpse switched to run stance")
			}
			if after := rec.Events()[before:]; len(after) != 0 {
				t.Errorf("corpse emitted %#v, want nothing", after)
			}
		})
	}
}

// TestConcurrentLethalHitsKillFolkOnce follows Creature.doDie, which runs
// its death once under the status lock (isDead check first): many
// goroutines landing lethal hits and kills on one mortal civilian NPC at
// once kill it exactly once — one call reports the kill, observers see one
// death and the corpse holds one decay registration, whose deadline is
// never pushed back.
func TestConcurrentLethalHitsKillFolkOnce(t *testing.T) {
	const callers = 32
	for round := range 20 {
		var adds atomic.Int64
		base := time.Unix(1_700_000_000, 0)
		// Every registration reads the clock once, and each read is a
		// second later, so a second registration would store a later
		// deadline.
		decay, err := task.NewDecay(nopFolkDecay{}, func() time.Time {
			return base.Add(time.Duration(adds.Add(1)) * time.Second)
		})
		if err != nil {
			t.Fatal(err)
		}
		f, rec := newMortalFolk(t, int32(round+1), decay)
		attacker := newFolkAttacker(t)

		var kills atomic.Int64
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := range callers {
			wg.Go(func() {
				<-start
				var killed bool
				if i%2 == 0 {
					killed = f.TakeDamage(1_000_000, attacker)
				} else {
					killed = f.Kill(attacker)
				}
				if killed {
					kills.Add(1)
				}
			})
		}
		close(start)
		wg.Wait()

		if got := kills.Load(); got != 1 {
			t.Fatalf("round %d: %d calls reported the kill, want 1", round, got)
		}
		if got := event.Count[event.Died](rec); got != 1 {
			t.Fatalf("round %d: %d Died events, want 1", round, got)
		}
		if got := adds.Load(); got != 1 {
			t.Fatalf("round %d: %d decay registrations, want 1", round, got)
		}
		want := base.Add(time.Second).Add(f.CorpseTime())
		if got, ok := decay.Deadline(f); !ok || !got.Equal(want) {
			t.Fatalf("round %d: decay deadline = %v (tracked %v), want %v", round, got, ok, want)
		}
		if got, ok := f.CorpseDeadline(); !ok || !got.Equal(want) {
			t.Fatalf("round %d: corpse deadline = %v (set %v), want %v", round, got, ok, want)
		}
		if hp := f.HP(); hp != 0 {
			t.Fatalf("round %d: HP = %v after death, want 0", round, hp)
		}
	}
}

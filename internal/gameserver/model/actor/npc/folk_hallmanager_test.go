package npc

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Reference: ClanHallManagerNpcAI. thinkIdle replaces the NPC idle: when
// more than 300000 ms passed since the last check (0 at first), it records
// the time and lands its support buff on itself; resetBuffCheckTime zeroes
// the time. thinkCast, for a final target with an acting player: a skill
// in reuse does nothing; NPC MP below the skill's mpConsume plus
// mpInitialConsume sends support-no_mana.htm; otherwise the cast runs and
// support-done.htm follows, each with the NPC's MP.

// newHallManagerRig is newFolkCastRig for a clan hall manager holding mp
// MP, its target dist units away.
func newHallManagerRig(t *testing.T, mp float64, dist int) *folkCastRig {
	t.Helper()
	inst, err := NewInstance(1, &Template{ID: 35384, TemplateID: 35384, Type: "ClanHallManagerNpc", Level: 70, HPMax: 2444, MPMax: mp, RunSpeed: 120, WalkSpeed: 50})
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFolk(inst)
	if err != nil {
		t.Fatal(err)
	}
	r := &folkCastRig{
		f: f, in: sim.NewInline(time.Unix(1_700_000_000, 0)),
		aiTask: &recordingFolkAI{}, events: &recordingSink{}, los: &switchLOS{},
		target: &hostileTarget{id: 2, playable: true},
	}
	r.control = &scriptedFolkControl{f: f}
	r.castAI = &scriptedFolkCastAI{control: r.control, canDesire: true, canCast: true, castRange: 100}
	w := world.New()
	if err := f.Attach(FolkRuntime{World: w, Queue: r.in.NewQueue("folk"), Sink: r.events, AI: r.aiTask, LOS: r.los}); err != nil {
		t.Fatal(err)
	}
	f.SetCaster(r.control, r.castAI)
	w.Spawn(f, 100, 0, 0, 0)
	w.Spawn(r.target, 100+dist, 0, 0, 0)
	r.tick(t)
	return r
}

func (r *folkCastRig) buffChecks() int {
	n := 0
	for _, ev := range r.events.events {
		if _, ok := ev.(event.HallManagerBuffCheck); ok {
			n++
		}
	}
	return n
}

func (r *folkCastRig) supportAnswers() []event.HallSupportCast {
	var out []event.HallSupportCast
	for _, ev := range r.events.events {
		if a, ok := ev.(event.HallSupportCast); ok {
			out = append(out, a)
		}
	}
	return out
}

// TestHallManagerBuffCheckEveryFiveMinutes: the first idle tick checks the
// buff, the next one only once more than five minutes passed, and a reset
// has the next idle tick check at once. The manager never goes to its walk
// stance while idling.
func TestHallManagerBuffCheckEveryFiveMinutes(t *testing.T) {
	t.Parallel()
	r := newHallManagerRig(t, 1000, 50)
	r.tick(t)
	if n := r.buffChecks(); n != 1 {
		t.Fatalf("checks after the first idle tick = %d, want 1", n)
	}
	r.in.Advance(5 * time.Minute)
	r.tick(t)
	if n := r.buffChecks(); n != 1 {
		t.Fatalf("checks at exactly five minutes = %d, want 1", n)
	}
	r.in.Advance(time.Millisecond)
	r.tick(t)
	if n := r.buffChecks(); n != 2 {
		t.Fatalf("checks past five minutes = %d, want 2", n)
	}
	r.f.ResetSupportBuffCheck()
	r.tick(t)
	if n := r.buffChecks(); n != 3 {
		t.Fatalf("checks after a reset = %d, want 3", n)
	}
	for _, ev := range r.events.events {
		if m, ok := ev.(event.MoveTypeChanged); ok && !m.Running {
			t.Fatal("idle clan hall manager switched to its walk stance")
		}
	}
}

// TestHallManagerSupportCast: with the MP the skill takes the manager casts
// and answers done with its MP; short of it, it casts nothing and answers
// no mana.
func TestHallManagerSupportCast(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		over     int
		wantCast bool
	}{
		{"enough mana", 0, true},
		{"short of mana", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newHallManagerRig(t, 1000, 50)
			mp := int(r.f.MPValue())
			if mp <= 0 {
				t.Fatalf("manager MP = %d, want some", mp)
			}
			// The skill takes exactly the manager's MP, or one more.
			r.castAI.skillMP = mp + tc.over
			r.desire(r.target, 4342, 1e6)
			r.tick(t)
			if got := len(r.castAI.casts) == 1; got != tc.wantCast {
				t.Fatalf("cast started = %v, want %v", got, tc.wantCast)
			}
			answers := r.supportAnswers()
			want := event.HallSupportCast{PlayerID: r.target.ObjectID(), NoMana: !tc.wantCast, MP: mp}
			if len(answers) != 1 || answers[0] != want {
				t.Fatalf("answers = %+v, want [%+v]", answers, want)
			}
		})
	}
}

// TestFolkSupportCastOnlyForHallManager: any other civilian NPC casts on a
// player without an answer.
func TestFolkSupportCastOnlyForHallManager(t *testing.T) {
	t.Parallel()
	r := newFolkCastRig(t, 50)
	r.castAI.skillMP = 1_000_000
	r.desire(r.target, 4380, 1e6)
	r.tick(t)
	if len(r.castAI.casts) != 1 {
		t.Fatalf("casts = %v, want one", r.castAI.casts)
	}
	if a := r.supportAnswers(); len(a) != 0 || r.buffChecks() != 0 {
		t.Fatalf("answers = %+v, checks = %d; want none", a, r.buffChecks())
	}
}

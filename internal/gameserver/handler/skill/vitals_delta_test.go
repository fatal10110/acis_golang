package skill

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// staleTarget is a skillTarget a hit reaches from a second queue the moment
// a handler first touches its vitals: every read returns the values from
// before that hit, so the handler reads, the hit lands, then the handler
// writes. A handler that writes an absolute value computed from its read
// erases the hit; one that applies a delta keeps it.
type staleTarget struct {
	*skillTarget
	other  *sim.Queue
	hit    func(*skillTarget)
	fired  bool
	hp, cp float64 // what the handler read
}

func (t *staleTarget) land() {
	if t.fired {
		return
	}
	t.fired = true
	t.hp, t.cp = t.skillTarget.hp, t.skillTarget.cp
	done := make(chan struct{})
	if !t.other.Post(func() { t.hit(t.skillTarget); close(done) }) {
		panic("hit queue closed")
	}
	<-done
}

func (t *staleTarget) HP() float64 { t.land(); return t.hp }
func (t *staleTarget) CP() float64 { t.land(); return t.cp }

// A write that comes before any read still lands after the hit.
func (t *staleTarget) AddCP(v float64) float64    { t.land(); return t.skillTarget.AddCP(v) }
func (t *staleTarget) ReduceCP(v float64) float64 { t.land(); return t.skillTarget.ReduceCP(v) }
func (t *staleTarget) ShiftHP(delta float64) bool { t.land(); return t.skillTarget.ShiftHP(delta) }

// interleave runs cast on one queue of a two-worker pool, the hits that
// targets take on another.
func interleave(t *testing.T, cast Cast, targets ...*staleTarget) {
	t.Helper()
	pool := sim.NewPool(2, zerolog.Nop())
	pool.Start(context.Background())
	t.Cleanup(func() { _ = pool.Stop(context.Background()) })
	caster, hitter := pool.NewQueue("caster"), pool.NewQueue("hitter")
	for _, target := range targets {
		target.other = hitter
		cast.Targets = append(cast.Targets, target)
	}
	done := make(chan struct{})
	if !caster.Post(func() { NewDefaultRegistry().Use(cast); close(done) }) {
		t.Fatal("caster queue closed")
	}
	<-done
	for i, target := range targets {
		if !target.fired {
			t.Fatalf("target %d: the handler never touched its vitals", i)
		}
	}
}

// TestVitalsHandlersKeepAHitLandingBetweenReadAndWrite pins that each
// handler changes vitals by a delta under the target's lock, so a hit from
// another queue between the handler's read and its write survives (#5085).
func TestVitalsHandlersKeepAHitLandingBetweenReadAndWrite(t *testing.T) {
	t.Parallel()
	cpHit := func(n float64) func(*skillTarget) { return func(s *skillTarget) { s.cp -= n } }
	hpHit := func(n float64) func(*skillTarget) { return func(s *skillTarget) { s.hp -= n } }

	t.Run("CP heal", func(t *testing.T) {
		t.Parallel()
		target := &staleTarget{skillTarget: &skillTarget{isPlayer: true, cp: 50, maxCP: 100}, hit: cpHit(30)}
		interleave(t, Cast{Caster: target, Skill: modelskill.Definition{SkillType: "COMBATPOINTHEAL", Power: 20}}, target)
		if target.skillTarget.cp != 40 {
			t.Fatalf("CP = %v, want 40 (50 - 30 hit + 20 heal)", target.skillTarget.cp)
		}
		if target.noticeAmount != 20 {
			t.Fatalf("restored notice = %d, want 20", target.noticeAmount)
		}
	})

	t.Run("CP heal reports what it applied", func(t *testing.T) {
		t.Parallel()
		// The read says 20 of 100 CP are missing; a hit takes 30 more, so all
		// 50 of the heal applies.
		target := &staleTarget{skillTarget: &skillTarget{isPlayer: true, cp: 80, maxCP: 100}, hit: cpHit(30)}
		interleave(t, Cast{Caster: target, Skill: modelskill.Definition{SkillType: "COMBATPOINTHEAL", Power: 50}}, target)
		if target.skillTarget.cp != 100 || target.noticeAmount != 50 {
			t.Fatalf("CP = %v, notice %d, want 100 and 50", target.skillTarget.cp, target.noticeAmount)
		}
	})

	t.Run("CP damage percent", func(t *testing.T) {
		t.Parallel()
		target := &staleTarget{skillTarget: &skillTarget{isPlayer: true, cp: 80, maxCP: 100}, hit: cpHit(30)}
		interleave(t, Cast{Caster: &skillTarget{}, Skill: modelskill.Definition{SkillType: "CPDAMPERCENT", Power: 35}}, target)
		if target.skillTarget.cp != 22 {
			t.Fatalf("CP = %v, want 22 (80 - 30 hit - 28 damage)", target.skillTarget.cp)
		}
	})

	t.Run("balance life", func(t *testing.T) {
		t.Parallel()
		hit := &staleTarget{skillTarget: &skillTarget{hp: 40, maxHP: 100}, hit: hpHit(15)}
		other := &staleTarget{skillTarget: &skillTarget{hp: 80, maxHP: 100}, hit: func(*skillTarget) {}}
		interleave(t, Cast{Caster: &skillTarget{}, Skill: modelskill.Definition{SkillType: "BALANCE_LIFE"}}, hit, other)
		if !almost(hit.skillTarget.hp, 45) || !almost(other.skillTarget.hp, 60) {
			t.Fatalf("HP = %v/%v, want 45/60 (40 - 15 hit + 20 share, 80 - 20 share)", hit.skillTarget.hp, other.skillTarget.hp)
		}
	})

	t.Run("real damage", func(t *testing.T) {
		t.Parallel()
		target := &staleTarget{skillTarget: &skillTarget{isPlayer: true, hp: 100, maxHP: 100}, hit: hpHit(30)}
		interleave(t, Cast{Caster: &skillTarget{}, Skill: modelskill.Definition{SkillType: "REAL_DAMAGE", Power: 10}}, target)
		if target.skillTarget.hp != 60 || target.skillTarget.dead {
			t.Fatalf("HP = %v dead=%v, want 60 alive (100 - 30 hit - 10)", target.skillTarget.hp, target.skillTarget.dead)
		}
	})

	t.Run("real damage kills through the hit", func(t *testing.T) {
		t.Parallel()
		caster := &skillTarget{}
		target := &staleTarget{skillTarget: &skillTarget{isPlayer: true, hp: 40, maxHP: 100}, hit: hpHit(20)}
		interleave(t, Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "REAL_DAMAGE", Power: 30}}, target)
		if !target.skillTarget.dead || target.diedBy != caster {
			t.Fatalf("dead=%v by %v, want killed by the caster (40 - 20 hit - 30)", target.skillTarget.dead, target.diedBy)
		}
	})
}

package network

import (
	"testing"
	"time"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

// fakeCubicTimer/fakeCubicClock give cubic.AfterFunc a deterministic,
// manually-advanced stand-in so a test can trigger a cubic's action tick
// without waiting on a real timer, matching cubic package's own runtime
// tests.
type fakeCubicTimer struct{ stopped bool }

func (t *fakeCubicTimer) Stop() bool { wasRunning := !t.stopped; t.stopped = true; return wasRunning }

type fakeCubicClock struct {
	scheduled []struct {
		fn    func()
		timer *fakeCubicTimer
	}
}

func (c *fakeCubicClock) after(_ time.Duration, fn func()) cubic.Timer {
	t := &fakeCubicTimer{}
	c.scheduled = append(c.scheduled, struct {
		fn    func()
		timer *fakeCubicTimer
	}{fn, t})
	return t
}

func (c *fakeCubicClock) fireLast() {
	if len(c.scheduled) > 0 {
		c.scheduled[len(c.scheduled)-1].fn()
	}
}

func newCubicTestLink(t testing.TB, clock *fakeCubicClock, stance *attackStanceRecorder, defs ...modelskill.Definition) *GameClientLink {
	t.Helper()
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable(defs))

	return &GameClientLink{
		skills:         skills,
		skillHandlers:  handlerskill.NewDefaultRegistry(),
		attackStance:   stance,
		cubicAfterFunc: clock.after,
		afterFunc:      func(_ time.Duration, fn func()) { fn() },
	}
}

// life cubic id (4051) heal skill; keep the granting SUMMON definition and
// the fired heal skill's definition minimal but distinct.
const (
	testLifeCubicGrantSkillID = 900
	testLifeCubicHealSkillID  = 4051
	testStormCubicGrantID     = 901
	testStormFireSkillID      = 4049
)

func TestFireCubic_LifeCubicSelfStartsAndFiresWithoutAttackStance(t *testing.T) {
	clock := &fakeCubicClock{}
	stance := &attackStanceRecorder{}
	l := newCubicTestLink(t, clock, stance,
		modelskill.Definition{
			ID: testLifeCubicGrantSkillID, Level: 1, SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Life),
			CubicActivationTime: 8, CubicActivationChance: 30, SummonTotalLifeTime: 1200000,
		},
		modelskill.Definition{ID: testLifeCubicHealSkillID, Level: 1, SkillType: "HEAL", Target: modelskill.TargetOne},
	)

	capture := &frameCapture{}
	live := newTestLivePlayer(t, 1, capture)
	// A huge raw MaxHP baseline with CurrentHP at 1 keeps the actual
	// HP-ratio well under 1.0 regardless of the derived-stat formula
	// CalcStat applies on top of the raw baseline.
	live.SetResourceValues(player.Resources{MaxHP: 1000, CurrentHP: 1, MaxMP: 30, CurrentMP: 30})
	live.SetRollSource(func(int) int { return 0 }) // always passes the heal-probability roll

	def, _ := l.skills.Definition(modelskill.Ref{ID: testLifeCubicGrantSkillID, Level: 1})
	l.syncCubicRuntime(live, cubic.Life, def)

	// Life Cubic self-starts (Action() called at grant), independent of
	// combat stance.
	if len(clock.scheduled) == 0 {
		t.Fatal("Life Cubic did not self-start its action tick on grant")
	}
	clock.fireLast()

	found := false
	for _, op := range frameOpcodes(capture.frames) {
		if op == serverpackets.OpcodeMagicSkillUse {
			found = true
		}
	}
	if !found {
		t.Fatalf("Life Cubic fire did not broadcast MagicSkillUse, frames = %v", frameOpcodes(capture.frames))
	}
}

func TestFireCubic_NonLifeCubicStopsWhenOwnerNotInAttackStance(t *testing.T) {
	clock := &fakeCubicClock{}
	stance := &attackStanceRecorder{} // live never added: not in attack stance
	l := newCubicTestLink(t, clock, stance,
		modelskill.Definition{
			ID: testStormCubicGrantID, Level: 1, SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Storm),
			CubicActivationTime: 8, CubicActivationChance: 100, SummonTotalLifeTime: 1200000,
		},
		modelskill.Definition{ID: testStormFireSkillID, Level: 1, SkillType: "MDAM", Target: modelskill.TargetOne},
	)

	capture := &frameCapture{}
	live := newTestLivePlayer(t, 1, capture)

	def, _ := l.skills.Definition(modelskill.Ref{ID: testStormCubicGrantID, Level: 1})
	l.syncCubicRuntime(live, cubic.Storm, def)
	// syncCubicRuntime always (re)schedules the disappear timer; only the
	// Life Cubic additionally starts its action tick immediately.
	if len(clock.scheduled) != 1 {
		t.Fatalf("scheduled after grant = %d, want 1 (disappear timer only, no self-started action tick)", len(clock.scheduled))
	}

	// Simulate the owner's combat-stance entry activating the cubic
	// (task.AttackStance.Add's cubic.Action() hook), then let the tick fire
	// while the owner is (per the stance fake) no longer in attack stance.
	runtimes := live.Cubics()
	if len(runtimes) != 1 {
		t.Fatalf("Cubics() = %d entries, want 1", len(runtimes))
	}
	runtimes[0].Action()
	clock.fireLast()

	for _, op := range frameOpcodes(capture.frames) {
		if op == serverpackets.OpcodeMagicSkillUse {
			t.Fatal("cubic fired MagicSkillUse while owner not in attack stance")
		}
	}
}

func TestFireCubic_NonLifeCubicFiresAgainstOwnerTargetInAttackStance(t *testing.T) {
	clock := &fakeCubicClock{}
	stance := &attackStanceRecorder{}
	l := newCubicTestLink(t, clock, stance,
		modelskill.Definition{
			ID: testStormCubicGrantID, Level: 1, SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Storm),
			CubicActivationTime: 8, CubicActivationChance: 100, SummonTotalLifeTime: 1200000,
		},
		modelskill.Definition{ID: testStormFireSkillID, Level: 1, SkillType: "MDAM", Target: modelskill.TargetOne},
	)

	capture := &frameCapture{}
	live := newTestLivePlayer(t, 1, capture)
	live.SetRollSource(func(int) int { return 0 }) // passes activation-chance roll, picks skill index 0

	target := newTestHostileNPC(t, 2)
	live.SetTargetTracked(target)
	stance.Add(live)

	def, _ := l.skills.Definition(modelskill.Ref{ID: testStormCubicGrantID, Level: 1})
	l.syncCubicRuntime(live, cubic.Storm, def)

	runtimes := live.Cubics()
	if len(runtimes) != 1 {
		t.Fatalf("Cubics() = %d entries, want 1", len(runtimes))
	}
	runtimes[0].Action()
	clock.fireLast()

	found := false
	for _, op := range frameOpcodes(capture.frames) {
		if op == serverpackets.OpcodeMagicSkillUse {
			found = true
		}
	}
	if !found {
		t.Fatalf("cubic in attack stance with a valid target did not broadcast MagicSkillUse, frames = %v", frameOpcodes(capture.frames))
	}
}

func TestFireCubic_DeadOwnerStopsInsteadOfFiring(t *testing.T) {
	clock := &fakeCubicClock{}
	stance := &attackStanceRecorder{}
	l := newCubicTestLink(t, clock, stance,
		modelskill.Definition{
			ID: testLifeCubicGrantSkillID, Level: 1, SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Life),
			CubicActivationTime: 8, CubicActivationChance: 30, SummonTotalLifeTime: 1200000,
		},
		modelskill.Definition{ID: testLifeCubicHealSkillID, Level: 1, SkillType: "HEAL", Target: modelskill.TargetOne},
	)

	capture := &frameCapture{}
	live := newTestLivePlayer(t, 1, capture)
	live.SetResourceValues(player.Resources{MaxHP: 1000, CurrentHP: 1, MaxMP: 30, CurrentMP: 30})
	live.SetRollSource(func(int) int { return 0 })

	def, _ := l.skills.Definition(modelskill.Ref{ID: testLifeCubicGrantSkillID, Level: 1})
	l.syncCubicRuntime(live, cubic.Life, def)
	live.Character.Die(nil)

	clock.fireLast()

	for _, op := range frameOpcodes(capture.frames) {
		if op == serverpackets.OpcodeMagicSkillUse {
			t.Fatal("dead owner's cubic fired MagicSkillUse, want stopped instead")
		}
	}
	if ids := live.Character.CubicIDs(); len(ids) != 0 {
		t.Fatalf("CubicIDs() after dead-owner tick = %v, want empty (cubic stopped)", ids)
	}
	if len(live.Cubics()) != 0 {
		t.Fatalf("Cubics() after dead-owner tick = %d, want 0 (runtime removed)", len(live.Cubics()))
	}
}

func TestLivePlayerStop_StopsLiveCubicRuntimes(t *testing.T) {
	clock := &fakeCubicClock{}
	stance := &attackStanceRecorder{}
	l := newCubicTestLink(t, clock, stance,
		modelskill.Definition{
			ID: testLifeCubicGrantSkillID, Level: 1, SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Life),
			CubicActivationTime: 8, CubicActivationChance: 30, SummonTotalLifeTime: 1200000,
		},
		modelskill.Definition{ID: testLifeCubicHealSkillID, Level: 1, SkillType: "HEAL", Target: modelskill.TargetOne},
	)

	capture := &frameCapture{}
	live := newTestLivePlayer(t, 1, capture)
	def, _ := l.skills.Definition(modelskill.Ref{ID: testLifeCubicGrantSkillID, Level: 1})
	l.syncCubicRuntime(live, cubic.Life, def)

	before := len(clock.scheduled)
	if before == 0 {
		t.Fatal("Life Cubic did not schedule any timer on grant")
	}

	live.Stop()

	// Every timer scheduled up to detach must be cancelled; a tick that
	// still fires after Stop() would keep the session's cubic alive past
	// logout.
	for i, s := range clock.scheduled[:before] {
		if !s.timer.stopped {
			t.Fatalf("scheduled[%d] not stopped after livePlayer.Stop()", i)
		}
	}
}

func TestCubicGrantedLevel(t *testing.T) {
	tests := []struct {
		name string
		def  modelskill.Definition
		want int
	}{
		{"regular skill passes level through", modelskill.Definition{ID: 10, Level: 5}, 5},
		{"Life Cubic for Beginners forces level 8", modelskill.Definition{ID: 4338, Level: 1}, 8},
		{"enchanted level above 100 collapses via the reference formula", modelskill.Definition{ID: 10, Level: 121}, 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cubicGrantedLevel(tt.def); got != tt.want {
				t.Fatalf("cubicGrantedLevel(%+v) = %d, want %d", tt.def, got, tt.want)
			}
		})
	}
}

type fakeCubicHealTarget struct {
	healable      bool
	effectiveness float64
	added         float64
}

func (f *fakeCubicHealTarget) CanBeHealed() bool { return f.healable }
func (f *fakeCubicHealTarget) AddHP(amount float64) float64 {
	f.added = amount
	return amount
}
func (f *fakeCubicHealTarget) HealEffectiveness() float64 { return f.effectiveness }

func TestApplyCubicHeal_FlatFormulaNoCasterStats(t *testing.T) {
	target := &fakeCubicHealTarget{healable: true, effectiveness: 150}
	applyCubicHeal(modelskill.Definition{Power: 200}, target)

	want := 200.0 * 150 / 100
	if target.added != want {
		t.Fatalf("AddHP amount = %v, want %v (power * effectiveness / 100, no caster stats)", target.added, want)
	}
}

func TestApplyCubicHeal_SkipsUnhealableTarget(t *testing.T) {
	target := &fakeCubicHealTarget{healable: false, effectiveness: 100}
	applyCubicHeal(modelskill.Definition{Power: 200}, target)

	if target.added != 0 {
		t.Fatalf("AddHP called on an unhealable target, amount = %v", target.added)
	}
}

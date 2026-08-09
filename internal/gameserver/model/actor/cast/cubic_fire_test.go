package cast

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestCubicGrantedLevel(t *testing.T) {
	tests := []struct {
		name string
		def  modelskill.Definition
		want int
	}{
		{"regular skill passes level through", modelskill.Definition{ID: 10, Level: 5}, 5},
		{"Life Cubic for Beginners forces level 8", modelskill.Definition{ID: 4338, Level: 1}, 8},
		{"enchanted level above 100 collapses via the reference formula", modelskill.Definition{ID: 10, Level: 121}, 11},
		{"non-exact-multiple truncates toward zero like Java int division", modelskill.Definition{ID: 10, Level: 125}, 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CubicGrantedLevel(tt.def); got != tt.want {
				t.Fatalf("CubicGrantedLevel(%+v) = %d, want %d", tt.def, got, tt.want)
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
	if !ApplyCubicHeal(200, target) {
		t.Fatal("ApplyCubicHeal() = false, want true (healable target)")
	}

	want := 200.0 * 150 / 100
	if target.added != want {
		t.Fatalf("AddHP amount = %v, want %v (power * effectiveness / 100, no caster stats)", target.added, want)
	}
}

func TestApplyCubicHeal_SkipsUnhealableTarget(t *testing.T) {
	target := &fakeCubicHealTarget{healable: false, effectiveness: 100}
	if ApplyCubicHeal(200, target) {
		t.Fatal("ApplyCubicHeal() = true for an unhealable target, want false")
	}
	if target.added != 0 {
		t.Fatalf("AddHP called on an unhealable target, amount = %v", target.added)
	}
}

// fakeCubicFireOwner is a minimal CubicFireOwner + Target implementer for
// domain-level target-selection tests.
type fakeCubicFireOwner struct {
	objectID int32
	x, y, z  int
	target   any
	rolls    []int
	rollIdx  int
	hp       int
	maxHP    float64
}

func (f *fakeCubicFireOwner) ObjectID() int32           { return f.objectID }
func (f *fakeCubicFireOwner) Position() (int, int, int) { return f.x, f.y, f.z }
func (f *fakeCubicFireOwner) Target() any               { return f.target }
func (f *fakeCubicFireOwner) CurrentHP() int            { return f.hp }
func (f *fakeCubicFireOwner) MaxHPValue() float64       { return f.maxHP }
func (f *fakeCubicFireOwner) Roll(n int) int {
	if f.rollIdx >= len(f.rolls) {
		return 0
	}
	v := f.rolls[f.rollIdx]
	f.rollIdx++
	if v >= n && n > 0 {
		return n - 1
	}
	return v
}

type fakeCubicTarget struct {
	objectID   int32
	x, y, z    int
	alikeDead  bool
	siegeGuard bool
}

func (f *fakeCubicTarget) ObjectID() int32           { return f.objectID }
func (f *fakeCubicTarget) Position() (int, int, int) { return f.x, f.y, f.z }
func (f *fakeCubicTarget) AlikeDead() bool           { return f.alikeDead }
func (f *fakeCubicTarget) SiegeGuard() bool          { return f.siegeGuard }

func TestDecideCubicFire_RejectsWhenActivationRollFails(t *testing.T) {
	owner := &fakeCubicFireOwner{rolls: []int{99}} // roll(100)=99 >= chance(30)
	_, _, ok := DecideCubicFire(owner, []int{4049}, 30)
	if ok {
		t.Fatal("DecideCubicFire() = true despite a failed activation roll")
	}
}

func TestDecideCubicFire_PicksTargetAndSkillOnSuccess(t *testing.T) {
	target := &fakeCubicTarget{objectID: 2, x: 100, y: 0, z: 0}
	owner := &fakeCubicFireOwner{objectID: 1, rolls: []int{0, 0}, target: target}
	skillID, got, ok := DecideCubicFire(owner, []int{4049, 4053}, 100)
	if !ok {
		t.Fatal("DecideCubicFire() = false, want true (roll passes, target in range)")
	}
	if skillID != 4049 {
		t.Fatalf("skillID = %d, want 4049 (first roll picks index 0)", skillID)
	}
	if got.ObjectID() != target.ObjectID() {
		t.Fatalf("target ObjectID = %d, want %d", got.ObjectID(), target.ObjectID())
	}
}

func TestDecideCubicFire_RejectsOutOfRangeTarget(t *testing.T) {
	target := &fakeCubicTarget{objectID: 2, x: 10000, y: 0, z: 0}
	owner := &fakeCubicFireOwner{objectID: 1, rolls: []int{0, 0}, target: target}
	_, _, ok := DecideCubicFire(owner, []int{4049}, 100)
	if ok {
		t.Fatal("DecideCubicFire() = true for a target far outside cubicMaxMagicRange")
	}
}

func TestDecideCubicFire_RejectsDeadTarget(t *testing.T) {
	target := &fakeCubicTarget{objectID: 2, alikeDead: true}
	owner := &fakeCubicFireOwner{objectID: 1, rolls: []int{0, 0}, target: target}
	_, _, ok := DecideCubicFire(owner, []int{4049}, 100)
	if ok {
		t.Fatal("DecideCubicFire() = true for an already-dead target")
	}
}

func TestDecideLifeCubicTarget_SkipsWhenAtFullHP(t *testing.T) {
	owner := &fakeCubicFireOwner{objectID: 1, hp: 100, maxHP: 100}
	_, ok := DecideLifeCubicTarget(owner)
	if ok {
		t.Fatal("DecideLifeCubicTarget() = true at full HP, want false")
	}
}

func TestDecideLifeCubicTarget_HealsSelfWhenRollPasses(t *testing.T) {
	owner := &fakeCubicFireOwner{objectID: 1, hp: 1, maxHP: 1000, rolls: []int{0}}
	target, ok := DecideLifeCubicTarget(owner)
	if !ok {
		t.Fatal("DecideLifeCubicTarget() = false despite low HP and a passing roll")
	}
	if target.ObjectID() != owner.ObjectID() {
		t.Fatalf("target ObjectID = %d, want owner's own %d (no-party fallback)", target.ObjectID(), owner.ObjectID())
	}
}

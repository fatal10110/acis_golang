package cast

import (
	"testing"
	"time"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/skill/skilltest"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target/targettest"
	modelactor "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect/effecttest"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from cubic_fire_test.go ----
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

// TestApplyCubicHeal_PlayerStatusOnlyWhenHPApplied pins Cubic.useHealSkill's
// addHp (Cubic.java:364-373, CreatureStatus.java:169-187): a player whose HP
// rose is sent its own status through setHp, whoever the healed player is;
// a player already at full HP gets none, and a summon or NPC target is
// left to its own AddHP, which republishes its status once. The heal still
// counts as landed either way, so the caller sends REJUVENATING_HP.
func TestApplyCubicHeal_PlayerStatusOnlyWhenHPApplied(t *testing.T) {
	t.Run("damaged player", func(t *testing.T) {
		target, rec := newHealTargetPlayer(500)
		if !ApplyCubicHeal(50, target) {
			t.Fatal("ApplyCubicHeal() = false, want true (healable target)")
		}
		if got := target.HP(); got != 550 {
			t.Fatalf("HP = %v, want 550", got)
		}
		if got := event.Count[event.VitalsChanged](rec); got != 1 {
			t.Fatalf("status updates = %d, want 1", got)
		}
	})
	t.Run("player at full HP", func(t *testing.T) {
		target, rec := newHealTargetPlayer(1000)
		if !ApplyCubicHeal(50, target) {
			t.Fatal("ApplyCubicHeal() = false, want true (healable target)")
		}
		if got := target.HP(); got != 1000 {
			t.Fatalf("HP = %v, want unchanged 1000", got)
		}
		if got := len(rec.Events()); got != 0 {
			t.Fatalf("events = %v, want none", rec.Events())
		}
	})
	t.Run("damaged summon", func(t *testing.T) {
		target, rec := newHealTargetServitor(t, 500)
		if !ApplyCubicHeal(50, target) {
			t.Fatal("ApplyCubicHeal() = false, want true (healable target)")
		}
		if got := target.HP(); got != 550 {
			t.Fatalf("HP = %v, want 550", got)
		}
		if got := event.Count[event.HPChanged](rec); got != 1 {
			t.Fatalf("HP updates = %d, want only the 1 its own AddHP publishes", got)
		}
	})
	t.Run("damaged npc", func(t *testing.T) {
		target, rec := newHealTargetNPC(t, 100)
		if !ApplyCubicHeal(50, target) {
			t.Fatal("ApplyCubicHeal() = false, want true (healable target)")
		}
		if got := target.CurrentHP(); got != 150 {
			t.Fatalf("HP = %d, want 150", got)
		}
		if got := event.Count[event.HPChanged](rec); got != 1 {
			t.Fatalf("HP updates = %d, want only the 1 its own AddHP publishes", got)
		}
	})
}

func TestApplyCubicHeal_FlatFormulaNoCasterStats(t *testing.T) {
	target, _ := newHealTargetPlayer(100)
	// A 150% heal effectiveness, as a heal-boosting buff would grant.
	target.AttachStatFuncs([]effect.Mod{{Stat: stat.HealEffectiveness, Op: effect.OpMul, Value: 1.5}})
	if got := target.HealEffectiveness(); got != 150 {
		t.Fatalf("HealEffectiveness() = %v, want 150", got)
	}
	if !ApplyCubicHeal(200, target) {
		t.Fatal("ApplyCubicHeal() = false, want true (healable target)")
	}

	want := 100 + 200.0*150/100
	if got := target.HP(); got != want {
		t.Fatalf("HP = %v, want %v (power * effectiveness / 100, no caster stats)", got, want)
	}
}

func TestApplyCubicHeal_SkipsUnhealableTarget(t *testing.T) {
	target, rec := newHealTargetPlayer(100)
	if !target.MarkDead() {
		t.Fatal("MarkDead() = false, want the player dead")
	}
	if ApplyCubicHeal(200, target) {
		t.Fatal("ApplyCubicHeal() = true for an unhealable target, want false")
	}
	if got := target.HP(); got != 0 {
		t.Fatalf("HP = %v, want the dead player's 0 unchanged", got)
	}
	if got := event.Count[event.VitalsChanged](rec); got != 0 {
		t.Fatalf("status updates = %d, want none", got)
	}
}

// fakeCubicEffectCaster and fakeCubicEffectTarget are the minimal
// handlerskill.Actor + cast.Target surface ApplyCubicEffect's dispatch
// needs. A fakeCubicEffectTarget with perfectBlock set blocks every skill
// perfectly, so an offensive continuous skill (DEBUFF/DOT/etc.) always
// fails the cubic landing roll — Cubic.useContinuousSkill's
// calcCubicSkillSuccess()==false branch (Cubic.java:439-444). Kept per
// docs/agents/test-strategy.md: a real creature blocks perfectly only on a
// shield-block roll against an equipped shield, and the perfect block is
// the fixed outcome this test needs, not the roll.
type fakeCubicEffectCaster struct {
	world.Presence
	skilltest.Creature
	id int32
}

func (f *fakeCubicEffectCaster) ObjectID() int32 { return f.id }

func (f *fakeCubicEffectCaster) Position() (int, int, int) { return 0, 0, 0 }

func (f *fakeCubicEffectCaster) Dead() bool { return false }

type fakeCubicEffectTarget struct {
	world.Presence
	skilltest.Creature
	id           int32
	list         *effect.List
	perfectBlock bool
}

func (f *fakeCubicEffectTarget) ShieldDefense(creature.FormulaActor, modelskill.Definition, bool) formulas.ShieldDefense {
	if f.perfectBlock {
		return formulas.ShieldPerfect
	}
	return formulas.ShieldFailed
}

func (f *fakeCubicEffectTarget) ObjectID() int32 { return f.id }

func (f *fakeCubicEffectTarget) Position() (int, int, int) { return 0, 0, 0 }

func (f *fakeCubicEffectTarget) Dead() bool { return false }

func (f *fakeCubicEffectTarget) EffectList() *effect.List { return f.list }

// TestApplyCubicEffect_FailedOffensiveContinuousRollReportsAttackFailed
// covers the issue-1570 gap: ApplyCubicEffect used to discard
// skills.UseResult's Result outright, so a failed offensive continuous
// landing roll never reached the caller and the cubic's owner never saw
// ATTACK_FAILED, unlike the reference's useContinuousSkill.
func TestApplyCubicEffect_FailedOffensiveContinuousRollReportsAttackFailed(t *testing.T) {
	registry := handlerskill.NewDefaultRegistry()
	caster := &fakeCubicEffectCaster{id: 1}
	target := &fakeCubicEffectTarget{id: 2, list: newTestList(nil), perfectBlock: true}

	def := modelskill.Definition{
		SkillType: "DEBUFF",
		Offensive: true,
		Debuff:    true,
		Effects:   []modelskill.EffectTemplate{{Name: "Buff", Time: 600}},
	}

	result := ApplyCubicEffect(registry, caster, def, 0, target, nil)

	if !result.Handled {
		t.Fatal("ApplyCubicEffect() Handled = false, want true (DEBUFF has a registered handler)")
	}
	if result.AttackFailed != 1 {
		t.Fatalf("ApplyCubicEffect() AttackFailed = %d, want 1", result.AttackFailed)
	}
}

// fakeCubicShotOwner is a cubic owner holding a charged blessed spiritshot
// and recording every shot-charge write. Kept per
// docs/agents/test-strategy.md: it pins that no charge is written at all,
// of any kind, which a real owner's weapon state cannot show for a write
// that leaves it as it was.
type fakeCubicShotOwner struct {
	fakeCubicEffectCaster
	blessed bool
	writes  []item.ShotKind
}

func (f *fakeCubicShotOwner) BlessedSpiritshotCharged() bool { return f.blessed }

func (f *fakeCubicShotOwner) SetChargedShot(kind item.ShotKind, charged bool) {
	f.writes = append(f.writes, kind)
	if kind == item.ShotBlessedSpirit {
		f.blessed = charged
	}
}

// TestApplyCubicEffect_ContinuousAndDisablerProcsKeepOwnerSpiritshot pins
// that a cubic's POISON/DEBUFF/DOT, PARALYZE/STUN/ROOT/AGGDAMAGE, MDAM and
// DRAIN procs read the owner's blessed-spiritshot charge without spending
// it, although the same skill types cast by a player discharge it
// (Cubic.java:378-480 never calls setChargedShot).
func TestApplyCubicEffect_ContinuousAndDisablerProcsKeepOwnerSpiritshot(t *testing.T) {
	registry := handlerskill.NewDefaultRegistry()
	for _, def := range []modelskill.Definition{
		{ID: 4052, SkillType: "POISON", Offensive: true, Debuff: true},
		{ID: 4053, SkillType: "DEBUFF", Offensive: true, Debuff: true},
		{ID: 4165, SkillType: "DOT", Offensive: true, Debuff: true},
		{ID: 4164, SkillType: "PARALYZE", Offensive: true},
		{ID: 4166, SkillType: "STUN", Offensive: true},
		{ID: 5116, SkillType: "ROOT", Offensive: true},
		{ID: 5115, SkillType: "AGGDAMAGE", Offensive: true},
		{ID: 4049, SkillType: "MDAM", Offensive: true, Power: 10},
		{ID: 4050, SkillType: "DRAIN", Offensive: true, Magic: true, Power: 10},
	} {
		t.Run(def.SkillType, func(t *testing.T) {
			owner := &fakeCubicShotOwner{fakeCubicEffectCaster: fakeCubicEffectCaster{id: 1}, blessed: true}
			target := &fakeCubicEffectTarget{id: 2, list: newTestList(nil)}

			if result := ApplyCubicEffect(registry, owner, def, 0, target, nil); !result.Handled {
				t.Fatalf("ApplyCubicEffect(%s) Handled = false, want true", def.SkillType)
			}
			if !owner.blessed || len(owner.writes) != 0 {
				t.Fatalf("owner blessed charge = %v, shot writes = %v; want charge kept and no writes", owner.blessed, owner.writes)
			}
		})
	}
}

// fakeCubicFireOwner is a minimal CubicFireOwner + Target implementer for
// domain-level target-selection tests. Kept per
// docs/agents/test-strategy.md: the production CubicFireOwner is the
// network package's adapter over a live player, which this package cannot
// import (network imports cast), and its scripted Roll is the RNG seam the
// activation, skill-pick and heal-band branches are driven through.
type fakeCubicFireOwner struct {
	objectID int32
	x, y, z  int
	target   world.Tracked
	rolls    []int
	rollIdx  int
	hp       int
	maxHP    float64
	attacker skilltarget.Actor
}

func (f *fakeCubicFireOwner) ObjectID() int32 { return f.objectID }

func (f *fakeCubicFireOwner) Attacker() skilltarget.Actor { return f.attacker }

func (f *fakeCubicFireOwner) Position() (int, int, int) { return f.x, f.y, f.z }

func (f *fakeCubicFireOwner) Target() world.Tracked { return f.target }

func (f *fakeCubicFireOwner) CurrentHP() int { return f.hp }

func (f *fakeCubicFireOwner) MaxHPValue() float64 { return f.maxHP }

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

// fakeCubicTarget is the owner's selected object. Kept per
// docs/agents/test-strategy.md: *npc.Hostile implements the same surface,
// but its no-force gate depends on the attacker only through
// self-exclusion, so it cannot show that the gate is asked about the
// owner's attacker (checkedBy) rather than some other actor.
type fakeCubicTarget struct {
	effecttest.Actor
	world.Presence
	objectID   int32
	x, y, z    int
	alikeDead  bool
	siegeGuard bool
	// refused makes the target one its attacker must force an attack on;
	// checkedBy is the attacker last checked.
	refused   bool
	checkedBy skilltarget.Actor
}

func (f *fakeCubicTarget) AttackableWithoutForceBy(caster skilltarget.Actor) bool {
	f.checkedBy = caster
	return !f.refused
}

func (f *fakeCubicTarget) ObjectID() int32 { return f.objectID }

func (*fakeCubicTarget) Kind() modelactor.Kind { return modelactor.KindNPC }

func (f *fakeCubicTarget) Position() (int, int, int) { return f.x, f.y, f.z }

func (f *fakeCubicTarget) AlikeDead() bool { return f.alikeDead }

func (f *fakeCubicTarget) SiegeGuard() bool { return f.siegeGuard }

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

// fakeCubicAttacker stands for a cubic's owner as an attacker; the test
// only compares it by identity. Kept per docs/agents/test-strategy.md.
type fakeCubicAttacker struct {
	targettest.Actor
	world.Presence
}

// TestDecideCubicFire_RejectsTargetNeedingForce pins the enemy gate: the
// owner's selection is fired at only when the owner may attack it without
// forcing, as checked against the owner itself.
func TestDecideCubicFire_RejectsTargetNeedingForce(t *testing.T) {
	attacker := &fakeCubicAttacker{}
	target := &fakeCubicTarget{objectID: 2, refused: true}
	owner := &fakeCubicFireOwner{objectID: 1, rolls: []int{0, 0}, target: target, attacker: attacker}
	if _, _, ok := DecideCubicFire(owner, []int{4049}, 100); ok {
		t.Fatal("DecideCubicFire() = true for a target the owner must force an attack on")
	}
	if target.checkedBy != attacker {
		t.Fatalf("checked against %v, want the owner's attacker", target.checkedBy)
	}
}

func TestDecideLifeCubicTarget_SkipsWhenAtFullHP(t *testing.T) {
	owner := &fakeCubicFireOwner{objectID: 1, hp: 100, maxHP: 100}
	_, ok := DecideLifeCubicTarget(owner, nil)
	if ok {
		t.Fatal("DecideLifeCubicTarget() = true at full HP, want false")
	}
}

func TestDecideLifeCubicTarget_HealsSelfWhenRollPasses(t *testing.T) {
	owner := &fakeCubicFireOwner{objectID: 1, hp: 1, maxHP: 1000, rolls: []int{0}}
	target, ok := DecideLifeCubicTarget(owner, nil)
	if !ok {
		t.Fatal("DecideLifeCubicTarget() = false despite low HP and a passing roll")
	}
	if target.ObjectID() != owner.ObjectID() {
		t.Fatalf("target ObjectID = %d, want owner's own %d (no-party fallback)", target.ObjectID(), owner.ObjectID())
	}
}

func (*fakeCubicEffectTarget) Kind() modelactor.Kind { return modelactor.KindNPC }

var _ handlerskill.Creature = (*fakeCubicEffectTarget)(nil)

// newTestList returns a list whose owner runs on its own inline queue, with
// the clock reading the wall time at creation.
func newTestList(owner effect.StatOwner) *effect.List {
	l := effect.NewList(owner)
	l.SetQueue(sim.NewInline(time.Now()).NewQueue("test"))
	return l
}

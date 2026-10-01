package npc

import (
	"math"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// ---- from hostile_effects_test.go ----
func TestHostileSkillSuccessInputUsesTemplateStatsAndCasterMagicAttack(t *testing.T) {
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 12, MAtk: 200})
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", Level: 10, MEN: 40, MDef: 50})
	def := skill.Definition{BaseLandRate: 50, EffectType: "ROOT", Magic: true, LevelDepend: 1}

	without, ok := target.SkillSuccessInput(caster, def, false, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput() ok = false")
	}
	with, ok := target.SkillSuccessInput(caster, def, true, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput(bss=true) ok = false")
	}

	if without.BaseChance != 50 {
		t.Fatalf("BaseChance = %v, want 50", without.BaseChance)
	}
	if want := math.Max(0, 2-math.Sqrt(statbonus.MENBonus[40])); !closeNPCFloat(without.StatModifier, want) {
		t.Fatalf("StatModifier = %v, want %v", without.StatModifier, want)
	}
	if want := without.MAtkModifier * 2; !closeNPCFloat(with.MAtkModifier, want) {
		t.Fatalf("MAtkModifier with bss = %v, want %v", with.MAtkModifier, want)
	}
	if want := 1.015; !closeNPCFloat(without.LevelModifier, want) {
		t.Fatalf("LevelModifier = %v, want %v", without.LevelModifier, want)
	}
}

func TestHostileInactiveRegionStopsAllEffects(t *testing.T) {
	hostile := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	hostile.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 1}, Template: skill.EffectTemplate{Name: "test"}})
	clock := driveHostile(hostile)

	hostile.OnInactiveRegion()
	clock.Run()

	if got := hostile.EffectList().All(); len(got) != 0 {
		t.Fatalf("effects after region deactivation = %d, want 0", len(got))
	}
}

// activityRecorder is an effect activity registry that records whether each
// list is registered.
type activityRecorder struct {
	mu     sync.Mutex
	active map[*effect.List]bool
}

func (r *activityRecorder) SetActive(l *effect.List, active bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active[l] = active
}

func (r *activityRecorder) registered(l *effect.List) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active[l]
}

// TestHostileDeathAndDecayStopEffects follows Creature.doDie and
// Npc.deleteMe: death ends every effect that does not last through death,
// running its exit hook; decay ends the rest and deregisters the list from
// the effect task.
func TestHostileDeathAndDecayStopEffects(t *testing.T) {
	rec := &activityRecorder{active: map[*effect.List]bool{}}
	hostile, err := NewHostile(&Instance{ObjectID: 1, Template: &Template{ID: 1, Type: "Monster"}, Kind: "Monster"},
		newHostileLive(t, effect.WithEnv(effect.Env{Activity: rec})), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	clock := driveHostile(hostile)
	exits := map[skill.ID]int{}
	add := func(meta effect.Skill, name string) *effect.Effect {
		e := &effect.Effect{Skill: meta, Template: skill.EffectTemplate{Name: name}, Effected: hostile, Effector: hostile}
		e.OnExit = func(e *effect.Effect) { exits[e.Skill.ID]++ }
		hostile.EffectList().Add(e)
		return e
	}
	add(effect.Skill{ID: 1}, "Buff")
	add(effect.Skill{ID: 2, Debuff: true}, "DamOverTime")
	lasting := add(effect.Skill{ID: 3, StayAfterDeath: true}, "Buff")
	list := hostile.EffectList()

	if !hostile.Die(nil, nil) {
		t.Fatal("Die() = false for a living NPC")
	}
	if held := list.All(); len(held) != 1 || held[0] != lasting {
		t.Fatalf("effects after death = %v, want only the one lasting through death", held)
	}
	if exits[1] != 1 || exits[2] != 1 || exits[3] != 0 {
		t.Fatalf("exit hooks after death = %v, want the buff and DoT once each", exits)
	}
	if !rec.registered(list) {
		t.Fatal("corpse holding a lasting effect is not registered with the effect task")
	}

	if !hostile.Decay(nil, nil) {
		t.Fatal("Decay() = false for a fresh corpse")
	}
	clock.Run()
	if held := list.All(); len(held) != 0 {
		t.Fatalf("effects after decay = %d, want 0", len(held))
	}
	if exits[3] != 1 {
		t.Fatalf("lasting effect exit hooks after decay = %d, want 1", exits[3])
	}
	if rec.registered(list) {
		t.Fatal("decayed NPC's effect list is still registered with the effect task")
	}
}

func TestHostileSkillSuccessInputAllowsIgnoreResistsWithoutCasterStats(t *testing.T) {
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster"})

	in, ok := target.SkillSuccessInput(nil, skill.Definition{
		BaseLandRate:  100,
		IgnoreResists: true,
	}, false, formulas.ShieldPerfect)
	if !ok {
		t.Fatal("SkillSuccessInput(ignore resists) ok = false")
	}
	if !in.IgnoreResists || in.BaseChance != 100 || in.Shield != formulas.ShieldPerfect {
		t.Fatalf("SkillSuccessInput(ignore resists) = %+v, want base chance, ignore flag, and shield preserved", in)
	}
}

func closeNPCFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// ---- from hostile_lethal_test.go ----
type deniedLethalCaster struct{ *Hostile }

func (deniedLethalCaster) CanGiveDamage() bool { return false }

func TestHostileLethalSurfaceBuildsInputAndAppliesOutcomes(t *testing.T) {
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 40, HPMax: 500})
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", Level: 45, HPMax: 500})
	def := skill.Definition{LethalChance1: 30, LethalChance2: 10, MagicLevel: 40}

	in, ok := target.LethalInput(caster, def)
	if !ok {
		t.Fatal("LethalInput() ok = false")
	}
	if in.Chance1 != 30 || in.Chance2 != 10 || in.MagicLevel != 40 || in.AttackerLevel != 40 || in.TargetLevel != 45 || in.LethalMul != 1 {
		t.Fatalf("LethalInput() = %+v, want skill fields and 40/45/1 actor values", in)
	}

	hp := target.MaxHPValue()
	target.SetHP(hp)
	target.ApplyLethalOutcome(formulas.LethalHalf, caster, def)
	if got := target.HP(); got != hp/2 {
		t.Fatalf("half lethal HP = %v, want %v", got, hp/2)
	}

	target.SetHP(hp)
	target.ApplyLethalOutcome(formulas.LethalFull, caster, def)
	if got := target.HP(); got != 1 {
		t.Fatalf("full lethal HP = %v, want 1", got)
	}
}

func TestHostileLethalInputRejectsGuardedDamage(t *testing.T) {
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 40, HPMax: 500})
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", Level: 45, HPMax: 500})
	def := skill.Definition{LethalChance1: 30}
	target.SetInvul(true)
	if _, ok := target.LethalInput(caster, def); ok {
		t.Fatal("LethalInput accepted an invulnerable hostile")
	}
	target.SetInvul(false)
	if _, ok := target.LethalInput(deniedLethalCaster{caster}, def); ok {
		t.Fatal("LethalInput accepted an attacker without damage permission")
	}
}

func TestHostileLethalableExcludesReferenceExceptions(t *testing.T) {
	for _, tt := range []struct {
		id   int
		want bool
	}{
		{id: 1, want: true},
		{id: 22215},
		{id: 22216},
		{id: 22217},
		{id: 35062},
		{id: 35410},
		{id: 35368},
		{id: 35375},
		{id: 35629},
	} {
		t.Run("npc", func(t *testing.T) {
			h := newCombatHostile(t, 1, &Template{ID: tt.id, Type: "Monster", HPMax: 100})
			if got := h.Lethalable(); got != tt.want {
				t.Fatalf("Lethalable() = %v, want %v for NPC %d", got, tt.want, tt.id)
			}
		})
	}
}

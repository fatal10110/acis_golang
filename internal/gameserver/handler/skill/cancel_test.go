package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// *player.Character must satisfy cancelTarget (and the sibling caster
// shapes that share the same Level() int requirement): it couldn't, before
// Character's persisted level field was renamed off of Level to make room
// for the method (see player.Character.CharLevel).
var (
	_ cancelTarget = (*player.Character)(nil)
	_ sowCaster    = (*player.Character)(nil)
	_ magicCaster  = (*player.Character)(nil)
)

type cancelFakeActor struct {
	dead  bool
	level int
	list  *effect.List
}

func newCancelFakeActor(level int) *cancelFakeActor {
	return &cancelFakeActor{level: level, list: effect.NewList(nil)}
}

func (a *cancelFakeActor) Dead() bool               { return a.dead }
func (a *cancelFakeActor) Level() int               { return a.level }
func (a *cancelFakeActor) EffectList() *effect.List { return a.list }

func addBuff(t *testing.T, actor *cancelFakeActor, tmpl modelskill.EffectTemplate, meta effect.Skill) *effect.Effect {
	t.Helper()
	e, err := effect.New(meta, tmpl)
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = actor
	actor.list.Add(e)
	return e
}

func hasEffect(list *effect.List, e *effect.Effect) bool {
	for _, cur := range list.All() {
		if cur == e {
			return true
		}
	}
	return false
}

func TestCancelNeverStripsToggleOrDebuffEffects(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newCancelFakeActor(40)

	toggle := addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 600}, effect.Skill{Toggle: true})
	debuff := addBuff(t, target, modelskill.EffectTemplate{Name: "Debuff", Time: 600}, effect.Skill{Debuff: true})

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "CANCEL", Power: 50, MaxNegatedEffects: 10, MagicLevel: 40},
		Targets: []any{target},
	})

	if !hasEffect(target.list, toggle) {
		t.Error("a toggle effect must never be stripped by CANCEL")
	}
	if !hasEffect(target.list, debuff) {
		t.Error("a debuff effect must never be stripped by CANCEL")
	}
}

func TestCancelNeverStripsNonCancellableEffectType(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newCancelFakeActor(40)

	blessing := addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 600, EffectType: "noblesse_blessing"}, effect.Skill{})

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "CANCEL", Power: 50, MaxNegatedEffects: 10, MagicLevel: 40},
		Targets: []any{target},
	})

	if !hasEffect(target.list, blessing) {
		t.Error("noblesse blessing must never be stripped by CANCEL")
	}
}

// A real ProtectionBlessing marker loaded from the datapack carries no
// effectType attribute, so its cancel-exemption must be resolved from the
// runtime kind the same way the attribute-tagged blessing above is.
func TestCancelNeverStripsProtectionBlessingMarkerEffect(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newCancelFakeActor(40)

	protection := addBuff(t, target, modelskill.EffectTemplate{Name: "ProtectionBlessing", Time: 600}, effect.Skill{})

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "CANCEL", Power: 50, MaxNegatedEffects: 10, MagicLevel: 40},
		Targets: []any{target},
	})

	if !hasEffect(target.list, protection) {
		t.Error("protection blessing must never be stripped by CANCEL")
	}
}

func TestMageBaneOnlyConsidersMatchingStackTypes(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newCancelFakeActor(40)

	unrelated := addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 600, StackType: "speed_up"}, effect.Skill{})

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "MAGE_BANE", Power: 50, MaxNegatedEffects: 10, MagicLevel: 40},
		Targets: []any{target},
	})

	if !hasEffect(target.list, unrelated) {
		t.Error("MAGE_BANE must never strip a stack type it doesn't cover")
	}
}

func TestCancelRefreshesCasterSelfEffect(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newCancelFakeActor(40)

	// A pre-existing self effect from the same skill should be dropped
	// before the fresh copy is applied, so re-casting doesn't stack it.
	stale := addBuff(t, caster, modelskill.EffectTemplate{Name: "Buff", Time: 600, Self: true}, effect.Skill{ID: 99})

	registry.Use(Cast{
		Caster: caster,
		Skill: modelskill.Definition{
			SkillType:   "CANCEL",
			ID:          99,
			SelfEffects: []modelskill.EffectTemplate{{Name: "Buff", Time: 600, Self: true}},
		},
	})

	if hasEffect(caster.list, stale) {
		t.Error("stale self effect should have been dropped before reapplying")
	}
	if len(caster.list.All()) != 1 {
		t.Fatalf("caster effect list = %d entries, want exactly 1 refreshed self effect", len(caster.list.All()))
	}
}

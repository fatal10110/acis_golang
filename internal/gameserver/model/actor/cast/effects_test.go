package cast

import (
	"testing"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// effectsActor is a minimal skilltarget.Creature usable as a non-player
// caster, proving the resolution path ApplyEffects drives doesn't require
// the live-player packet-handling type the player cast flow uses.
type effectsActor struct {
	id       int32
	x, y, z  int
	category skilltarget.Category
	dead     bool
	corpse   bool
	summon   *effectsActor
}

func (a *effectsActor) ObjectID() int32                { return a.id }
func (a *effectsActor) Position() (int, int, int)      { return a.x, a.y, a.z }
func (a *effectsActor) Heading() int                   { return 0 }
func (a *effectsActor) Dead() bool                     { return a.dead }
func (a *effectsActor) Category() skilltarget.Category { return a.category }

func (a *effectsActor) AttackableBy(skilltarget.Creature) bool             { return true }
func (a *effectsActor) AttackableWithoutForceBy(skilltarget.Creature) bool { return true }
func (a *effectsActor) HasCorpse() bool                                    { return a.corpse }

func (a *effectsActor) Summon() (skilltarget.Creature, bool) {
	if a.summon == nil {
		return nil, false
	}
	return a.summon, true
}

// effectsKnown is a fixed roster used as the radius-scan source for
// area/aura target handlers under test.
type effectsKnown []skilltarget.Creature

func (k effectsKnown) ForEachKnownCreatureInRadius(anchor skilltarget.Creature, _ int, fn func(skilltarget.Creature)) {
	for _, c := range k {
		if c.ObjectID() == anchor.ObjectID() {
			continue
		}
		fn(c)
	}
}

// recordingSkillHandler records every Cast it receives instead of applying
// any actual skill logic, so tests can assert on exactly what ApplyEffects
// resolved and handed off.
type recordingSkillHandler struct {
	skillTypes []string
	calls      []handlerskill.Cast
	result     handlerskill.Result
}

func (h *recordingSkillHandler) Types() []string { return h.skillTypes }

func (h *recordingSkillHandler) Use(c handlerskill.Cast) { h.calls = append(h.calls, c) }

func (h *recordingSkillHandler) UseResult(c handlerskill.Cast) handlerskill.Result {
	h.Use(c)
	return h.result
}

func newEffectHandlers(known skilltarget.Known, skillType string, rec *recordingSkillHandler) EffectHandlers {
	rec.skillTypes = []string{skillType}
	return EffectHandlers{
		Targets: skilltarget.NewRegistry(known),
		Skills:  handlerskill.NewRegistry(rec),
	}
}

func TestApplyEffectsResultCarriesSkillHandlerAttackFailed(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryPlayable}
	rec := &recordingSkillHandler{result: handlerskill.Result{AttackFailed: 2}}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 99, Target: modelskill.TargetSelf, SkillType: "DUMMY"}

	result := ApplyEffectsResult(handlers, caster, caster, def)
	if !result.Handled {
		t.Fatal("ApplyEffectsResult() handled = false, want true")
	}
	if result.AttackFailed != 2 {
		t.Fatalf("AttackFailed = %d, want 2", result.AttackFailed)
	}
}

func TestApplyEffectsAreaTargetReachesEveryAffectedCreature(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryAttackable}
	selected := &effectsActor{id: 2, x: 10, category: skilltarget.CategoryPlayable}
	bystander := &effectsActor{id: 3, x: 20, category: skilltarget.CategoryPlayable}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{caster, selected, bystander}, "DUMMY", rec)
	def := modelskill.Definition{ID: 100, Target: modelskill.TargetArea, Offensive: true, Radius: 900, SkillType: "DUMMY"}

	if !ApplyEffects(handlers, caster, selected, def) {
		t.Fatal("ApplyEffects(area) = false, want true")
	}
	if len(rec.calls) != 1 {
		t.Fatalf("skill handler calls = %d, want 1", len(rec.calls))
	}
	if got := rec.calls[0].Caster; got != any(caster) {
		t.Fatalf("recorded caster = %v, want %v", got, caster)
	}
	if len(rec.calls[0].Targets) != 2 {
		t.Fatalf("recorded targets = %d, want 2 (selected + bystander)", len(rec.calls[0].Targets))
	}
}

func TestApplyEffectsAuraTargetSweepsRadiusAroundCaster(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryAttackable}
	nearby := &effectsActor{id: 2, category: skilltarget.CategoryPlayable}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{caster, nearby}, "DUMMY", rec)
	def := modelskill.Definition{ID: 101, Target: modelskill.TargetAura, Radius: 300, SkillType: "DUMMY"}

	// Aura skills have no selected target: the caster is both the anchor
	// and the resolved target.
	if !ApplyEffects(handlers, caster, nil, def) {
		t.Fatal("ApplyEffects(aura) = false, want true")
	}
	if len(rec.calls) != 1 || len(rec.calls[0].Targets) != 1 {
		t.Fatalf("recorded call = %+v, want one call with one target", rec.calls)
	}
	if rec.calls[0].Targets[0] != any(nearby) {
		t.Fatalf("recorded target = %v, want %v", rec.calls[0].Targets[0], nearby)
	}
}

func TestApplyEffectsCorpseMobTargetRequiresPendingCorpse(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryPlayable}
	corpse := &effectsActor{id: 2, category: skilltarget.CategoryAttackable, dead: true, corpse: true}
	live := &effectsActor{id: 3, category: skilltarget.CategoryAttackable, corpse: false}
	def := modelskill.Definition{ID: 102, Target: modelskill.TargetCorpseMob, SkillType: "SWEEP"}

	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "SWEEP", rec)
	if !ApplyEffects(handlers, caster, corpse, def) {
		t.Fatal("ApplyEffects(corpse mob, has corpse) = false, want true")
	}
	if len(rec.calls) != 1 || len(rec.calls[0].Targets) != 1 || rec.calls[0].Targets[0] != any(corpse) {
		t.Fatalf("recorded call = %+v, want one call targeting the corpse", rec.calls)
	}

	rec2 := &recordingSkillHandler{}
	handlers2 := newEffectHandlers(effectsKnown{}, "SWEEP", rec2)
	if ApplyEffects(handlers2, caster, live, def) {
		t.Fatal("ApplyEffects(corpse mob, no corpse) = true, want false")
	}
	if len(rec2.calls) != 0 {
		t.Fatalf("skill handler calls = %d, want 0 for a target with no corpse", len(rec2.calls))
	}
}

func TestApplyEffectsSummonTargetResolvesCasterOwnedSummon(t *testing.T) {
	summon := &effectsActor{id: 2, category: skilltarget.CategoryPlayable}
	caster := &effectsActor{id: 1, category: skilltarget.CategoryPlayable, summon: summon}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 103, Target: modelskill.TargetSummon, SkillType: "DUMMY"}

	if !ApplyEffects(handlers, caster, nil, def) {
		t.Fatal("ApplyEffects(summon) = false, want true")
	}
	if len(rec.calls) != 1 || len(rec.calls[0].Targets) != 1 || rec.calls[0].Targets[0] != any(summon) {
		t.Fatalf("recorded call = %+v, want one call targeting the caster's summon", rec.calls)
	}

	// A caster without a summon must not reach the skill handler at all.
	rec2 := &recordingSkillHandler{}
	handlers2 := newEffectHandlers(effectsKnown{}, "DUMMY", rec2)
	summonless := &effectsActor{id: 4, category: skilltarget.CategoryPlayable}
	if ApplyEffects(handlers2, summonless, nil, def) {
		t.Fatal("ApplyEffects(summon, no summon) = true, want false")
	}
	if len(rec2.calls) != 0 {
		t.Fatalf("skill handler calls = %d, want 0 for a caster without a summon", len(rec2.calls))
	}
}

func TestApplyEffectsUnresolvedTargetTypeIsNoop(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryPlayable}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 104, Target: modelskill.TargetEnemySummon, SkillType: "DUMMY"}

	if ApplyEffects(handlers, caster, nil, def) {
		t.Fatal("ApplyEffects(unregistered target type) = true, want false")
	}
	if len(rec.calls) != 0 {
		t.Fatalf("skill handler calls = %d, want 0", len(rec.calls))
	}
}

func TestApplyEffectsNilRegistriesAreNoop(t *testing.T) {
	caster := &effectsActor{id: 1, category: skilltarget.CategoryPlayable}
	def := modelskill.Definition{ID: 105, Target: modelskill.TargetSelf, SkillType: "DUMMY"}

	if ApplyEffects(EffectHandlers{}, caster, nil, def) {
		t.Fatal("ApplyEffects(zero-value handlers) = true, want false")
	}
	if ApplyEffects(EffectHandlers{}, "not a creature", nil, def) {
		t.Fatal("ApplyEffects(non-Creature caster) = true, want false")
	}
}

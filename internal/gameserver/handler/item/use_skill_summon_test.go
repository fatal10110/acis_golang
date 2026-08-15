package item

import (
	"testing"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// fakeSummon is a minimal skilltarget.Creature usable as the herb-mirror
// destination, proving the mirror path reuses the same ApplyEffects surface
// any caster drives rather than a servitor-specific one.
type fakeSummon struct {
	id int32
}

func (s *fakeSummon) ObjectID() int32                { return s.id }
func (s *fakeSummon) Position() (int, int, int)      { return 0, 0, 0 }
func (s *fakeSummon) Heading() int                   { return 0 }
func (s *fakeSummon) Dead() bool                     { return false }
func (s *fakeSummon) Category() skilltarget.Category { return skilltarget.CategoryPlayable }

var _ actorcast.Target = (*fakeSummon)(nil)

type recordingSummonHandler struct {
	calls []handlerskill.Cast
}

func (h *recordingSummonHandler) Types() []string { return []string{"DUMMY"} }
func (h *recordingSummonHandler) Use(c handlerskill.Cast) {
	h.calls = append(h.calls, c)
}

func TestUseMirrorsHerbEffectOntoSummon(t *testing.T) {
	def := modelskill.Definition{ID: 100, Level: 1, Potion: true, Target: modelskill.TargetSelf, SkillType: "DUMMY"}
	rec := &recordingSummonHandler{}
	effects := actorcast.EffectHandlers{
		Targets: skilltarget.NewRegistry(noKnownCreatures{}),
		Skills:  handlerskill.NewRegistry(rec),
	}

	t.Run("herb with an active summon mirrors onto it", func(t *testing.T) {
		rec.calls = nil
		caster := &fakeCaster{}
		summon := &fakeSummon{id: 2}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, def, caster, &fakeDestroyer{}, false)
		req.Effects = effects
		req.Summon = summon

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied", res.Outcome)
		}
		res.Apply()
		// caster and summon both satisfy skilltarget.Creature, so both
		// their own effect application (caster) and the mirror (summon)
		// record a call.
		if len(rec.calls) != 2 {
			t.Fatalf("skill handler calls = %d, want 2 (caster + summon mirror)", len(rec.calls))
		}
		if !mirroredTo(rec.calls, summon) {
			t.Fatalf("recorded calls = %v, want one with the summon as caster", rec.calls)
		}
	})

	t.Run("herb with no active summon does not mirror", func(t *testing.T) {
		rec.calls = nil
		caster := &fakeCaster{}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, def, caster, &fakeDestroyer{}, false)
		req.Effects = effects
		req.Summon = nil

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied", res.Outcome)
		}
		res.Apply()
		if len(rec.calls) != 1 {
			t.Fatalf("skill handler calls = %d, want 1 (caster only, no summon to mirror onto)", len(rec.calls))
		}
	})

	t.Run("herb used by a pet does not mirror onto its own summon field", func(t *testing.T) {
		rec.calls = nil
		caster := &fakeCaster{}
		summon := &fakeSummon{id: 2}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, def, caster, &fakeDestroyer{}, true)
		req.Effects = effects
		req.Summon = summon

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied", res.Outcome)
		}
		res.Apply()
		if len(rec.calls) != 1 || mirroredTo(rec.calls, summon) {
			t.Fatalf("recorded calls = %v, want caster only (IsPet caster must not mirror)", rec.calls)
		}
	})

	t.Run("non-herb potion does not mirror even with an active summon", func(t *testing.T) {
		rec.calls = nil
		caster := &fakeCaster{}
		summon := &fakeSummon{id: 2}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, def, caster, &fakeDestroyer{}, false)
		req.Effects = effects
		req.Summon = summon

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied", res.Outcome)
		}
		res.Apply()
		if len(rec.calls) != 1 || mirroredTo(rec.calls, summon) {
			t.Fatalf("recorded calls = %v, want caster only (non-herb must not mirror)", rec.calls)
		}
	})
}

func mirroredTo(calls []handlerskill.Cast, summon *fakeSummon) bool {
	for _, c := range calls {
		if c.Caster == any(summon) {
			return true
		}
	}
	return false
}

type noKnownCreatures struct{}

func (noKnownCreatures) ForEachKnownCreatureInRadius(skilltarget.Creature, int, func(skilltarget.Creature)) {
}

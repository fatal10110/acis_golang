package item

import (
	"testing"
	"time"

	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type fakeCaster struct {
	disabled     map[int32]bool
	disableCalls int
	reuseCalls   int
}

func (f *fakeCaster) ObjectID() int32              { return 1 }
func (f *fakeCaster) Position() (int, int, int)    { return 0, 0, 0 }
func (f *fakeCaster) SkillDisabled(key int32) bool { return f.disabled != nil && f.disabled[key] }
func (f *fakeCaster) DisableSkill(key int32, d time.Duration) {
	f.disableCalls++
}
func (f *fakeCaster) AddSkillReuse(ref modelskill.Ref, key int32, d time.Duration) {
	f.reuseCalls++
}

type fakeDefinitions struct {
	def modelskill.Definition
}

func (f fakeDefinitions) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	if ref.ID != f.def.ID || ref.Level != f.def.Level {
		return modelskill.Definition{}, false
	}
	return f.def, true
}

type fakeDestroyer struct {
	calls int
	fail  bool
}

func (f *fakeDestroyer) DestroyItem(inv *itemcontainer.Inventory, objectID int32, count int) (invops.Result, bool) {
	f.calls++
	if f.fail {
		return invops.Result{}, false
	}
	return invops.Result{}, true
}

func newUseRequest(t *testing.T, handler string, etcType modelitem.EtcItemType, def modelskill.Definition, caster *fakeCaster, destroyer *fakeDestroyer, isPet bool) UseRequest {
	t.Helper()
	tmpl := &modelitem.Template{
		ID:   1,
		Kind: modelitem.KindEtcItem,
		EtcItem: &modelitem.EtcItemDetail{
			Type:    etcType,
			Handler: handler,
		},
		AttachedSkills: []modelitem.SkillRef{{ID: int32(def.ID), Level: int32(def.Level)}},
	}
	table := modelitem.NewTable([]*modelitem.Template{tmpl})
	inv := itemcontainer.NewPlayerInventory(2, table)
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 1}

	return UseRequest{
		Caster:      caster,
		Inventory:   inv,
		Item:        inst,
		Definitions: fakeDefinitions{def: def},
		Effects:     actorcast.EffectHandlers{},
		Destroyer:   destroyer,
		IsPet:       isPet,
	}
}

func TestUse(t *testing.T) {
	potion := modelskill.Definition{ID: 100, Level: 1, Potion: true, ReuseDelay: 0}

	t.Run("potion consumes one unit", func(t *testing.T) {
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, destroyer, false)

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied", res.Outcome)
		}
		if destroyer.calls != 1 {
			t.Fatalf("DestroyItem calls = %d, want 1", destroyer.calls)
		}
	})

	t.Run("herb applies without consuming", func(t *testing.T) {
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, potion, caster, destroyer, false)

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied", res.Outcome)
		}
		if destroyer.calls != 0 {
			t.Fatalf("DestroyItem calls = %d, want 0 (herb must not consume)", destroyer.calls)
		}
	})

	t.Run("herb not enough items never rejects (no consume attempted)", func(t *testing.T) {
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{fail: true}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, potion, caster, destroyer, false)

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied", res.Outcome)
		}
	})

	t.Run("elixir applied for a player caster", func(t *testing.T) {
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{}
		req := newUseRequest(t, ElixirsHandler, modelitem.EtcItemElixir, potion, caster, destroyer, false)

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied", res.Outcome)
		}
		if destroyer.calls != 1 {
			t.Fatalf("DestroyItem calls = %d, want 1", destroyer.calls)
		}
	})

	t.Run("elixir rejects a pet caster", func(t *testing.T) {
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{}
		req := newUseRequest(t, ElixirsHandler, modelitem.EtcItemElixir, potion, caster, destroyer, true)

		res := Use(req)

		if res.Outcome != PetRejected {
			t.Fatalf("Outcome = %v, want PetRejected", res.Outcome)
		}
		if destroyer.calls != 0 {
			t.Fatalf("DestroyItem calls = %d, want 0 (rejected before consume)", destroyer.calls)
		}
	})

	t.Run("plain ItemSkills item ignores IsPet", func(t *testing.T) {
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, destroyer, true)

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied (ItemSkillsHandler doesn't gate on IsPet)", res.Outcome)
		}
	})

	t.Run("reuse rejected before consume", func(t *testing.T) {
		key := actorcast.ReuseKey(potion)
		caster := &fakeCaster{disabled: map[int32]bool{key: true}}
		destroyer := &fakeDestroyer{}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, destroyer, false)

		res := Use(req)

		if res.Outcome != ReuseRejected {
			t.Fatalf("Outcome = %v, want ReuseRejected", res.Outcome)
		}
		if destroyer.calls != 0 {
			t.Fatalf("DestroyItem calls = %d, want 0", destroyer.calls)
		}
	})

	t.Run("unrelated handler not handled", func(t *testing.T) {
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{}
		req := newUseRequest(t, "SomeOtherHandler", modelitem.EtcItemNone, potion, caster, destroyer, false)

		res := Use(req)

		if res.Outcome != NotHandled {
			t.Fatalf("Outcome = %v, want NotHandled", res.Outcome)
		}
	})
}

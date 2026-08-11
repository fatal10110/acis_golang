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
	disabled             map[int32]bool
	disableCalls         int
	reuseCalls           int
	shortBuffTaskSkillID int32
	flying               bool
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
func (f *fakeCaster) ShortBuffTaskSkillID() int32 { return f.shortBuffTaskSkillID }
func (f *fakeCaster) IsFlying() bool              { return f.flying }

type fakeDefinitions struct {
	def modelskill.Definition
}

func (f fakeDefinitions) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	if ref.ID != f.def.ID || ref.Level != f.def.Level {
		return modelskill.Definition{}, false
	}
	return f.def, true
}

type fakeDefinitionTable map[modelskill.Ref]modelskill.Definition

func (f fakeDefinitionTable) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	def, ok := f[ref]
	return def, ok
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
		ID:       1,
		Kind:     modelitem.KindEtcItem,
		Tradable: true,
		EtcItem: &modelitem.EtcItemDetail{
			Type: etcType, Handler: handler, SharedReuseGroup: -1,
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

	t.Run("herb with a summon reports it as MirroredSummon", func(t *testing.T) {
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, potion, caster, destroyer, false)
		summon := &fakeCaster{}
		req.Summon = summon

		res := Use(req)

		if res.MirroredSummon != summon {
			t.Fatalf("MirroredSummon = %v, want summon", res.MirroredSummon)
		}
	})

	t.Run("herb with no summon reports no MirroredSummon", func(t *testing.T) {
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, potion, caster, destroyer, false)

		res := Use(req)

		if res.MirroredSummon != nil {
			t.Fatalf("MirroredSummon = %v, want nil", res.MirroredSummon)
		}
	})

	t.Run("potion (non-herb) with a summon does not mirror", func(t *testing.T) {
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, destroyer, false)
		req.Summon = &fakeCaster{}

		res := Use(req)

		if res.MirroredSummon != nil {
			t.Fatalf("MirroredSummon = %v, want nil (non-herb never mirrors)", res.MirroredSummon)
		}
	})

	t.Run("herb used by a pet caster does not mirror", func(t *testing.T) {
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, potion, caster, destroyer, true)
		req.Summon = &fakeCaster{}

		res := Use(req)

		if res.MirroredSummon != nil {
			t.Fatalf("MirroredSummon = %v, want nil (pet caster never mirrors)", res.MirroredSummon)
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

	t.Run("pet rejects non-tradable herb", func(t *testing.T) {
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, potion, caster, destroyer, true)
		tmpl, _ := req.Inventory.Templates().Get(req.Item.TemplateID)
		tmpl.Tradable = false

		res := Use(req)

		if res.Outcome != PetRejected {
			t.Fatalf("Outcome = %v, want PetRejected", res.Outcome)
		}
		if destroyer.calls != 0 {
			t.Fatalf("DestroyItem calls = %d, want 0", destroyer.calls)
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

	t.Run("reports shared reuse group and the longer of skill/item reuse delay", func(t *testing.T) {
		def := modelskill.Definition{ID: 101, Level: 1, Potion: true, ReuseDelay: 1000}
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{}
		tmpl := &modelitem.Template{
			ID:   1,
			Kind: modelitem.KindEtcItem,
			EtcItem: &modelitem.EtcItemDetail{
				Type: modelitem.EtcItemPotion, Handler: ItemSkillsHandler,
				ReuseDelay: 3000, SharedReuseGroup: 7,
			},
			AttachedSkills: []modelitem.SkillRef{{ID: int32(def.ID), Level: int32(def.Level)}},
		}
		table := modelitem.NewTable([]*modelitem.Template{tmpl})
		inv := itemcontainer.NewPlayerInventory(2, table)
		req := UseRequest{
			Caster: caster, Inventory: inv, Item: &modelitem.Instance{ObjectID: 10, TemplateID: 1},
			Definitions: fakeDefinitions{def: def}, Effects: actorcast.EffectHandlers{}, Destroyer: destroyer,
		}

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied", res.Outcome)
		}
		if res.SharedReuseGroup != 7 {
			t.Fatalf("SharedReuseGroup = %d, want 7", res.SharedReuseGroup)
		}
		if res.ReuseMillis != 3000 {
			t.Fatalf("ReuseMillis = %d, want 3000 (item's 3000 > skill's 1000)", res.ReuseMillis)
		}
	})

	t.Run("no shared reuse group reports -1", func(t *testing.T) {
		caster := &fakeCaster{}
		destroyer := &fakeDestroyer{}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, destroyer, false)

		res := Use(req)

		if res.SharedReuseGroup != -1 {
			t.Fatalf("SharedReuseGroup = %d, want -1 (template default)", res.SharedReuseGroup)
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

func TestUseAll(t *testing.T) {
	first := modelskill.Definition{ID: 100, Level: 1, Potion: true, ReuseDelay: 1}
	second := modelskill.Definition{ID: 101, Level: 1, Potion: true, ReuseDelay: 1}

	t.Run("applies each attached instant skill in order", func(t *testing.T) {
		caster := &fakeCaster{}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, first, caster, &fakeDestroyer{}, false)
		tmpl, _ := req.Inventory.Templates().Get(req.Item.TemplateID)
		tmpl.AttachedSkills = []modelitem.SkillRef{{ID: int32(first.ID), Level: int32(first.Level)}, {ID: int32(second.ID), Level: int32(second.Level)}}
		req.Definitions = fakeDefinitionTable{
			{ID: first.ID, Level: first.Level}:   first,
			{ID: second.ID, Level: second.Level}: second,
		}

		results := UseAll(req)

		if len(results) != 2 || results[0].Skill.ID != first.ID || results[1].Skill.ID != second.ID {
			t.Fatalf("results = %#v, want both skills in attached order", results)
		}
		if caster.reuseCalls != 2 {
			t.Fatalf("AddSkillReuse calls = %d, want 2", caster.reuseCalls)
		}
	})

	t.Run("stops at the first reuse-disabled skill", func(t *testing.T) {
		caster := &fakeCaster{disabled: map[int32]bool{actorcast.ReuseKey(first): true}}
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, first, caster, &fakeDestroyer{}, false)
		tmpl, _ := req.Inventory.Templates().Get(req.Item.TemplateID)
		tmpl.AttachedSkills = []modelitem.SkillRef{{ID: int32(first.ID), Level: int32(first.Level)}, {ID: int32(second.ID), Level: int32(second.Level)}}
		req.Definitions = fakeDefinitionTable{
			{ID: first.ID, Level: first.Level}:   first,
			{ID: second.ID, Level: second.Level}: second,
		}

		results := UseAll(req)

		if len(results) != 1 || results[0].Outcome != ReuseRejected || results[0].Skill.ID != first.ID {
			t.Fatalf("results = %#v, want first skill reuse rejection only", results)
		}
		if caster.reuseCalls != 0 {
			t.Fatalf("AddSkillReuse calls = %d, want 0", caster.reuseCalls)
		}
	})
}

func TestUseAllStopsWhenSkillConditionFails(t *testing.T) {
	first := modelskill.Definition{
		ID: 100, Level: 1, Potion: true,
		Conditions: []modelskill.ConditionClause{{
			Root: modelskill.Condition{Kind: "player", Attrs: map[string]string{"flying": "false"}},
		}},
	}
	second := modelskill.Definition{ID: 101, Level: 1, Potion: true}
	for _, isPet := range []bool{false, true} {
		t.Run(map[bool]string{false: "player herb", true: "pet herb"}[isPet], func(t *testing.T) {
			caster := &fakeCaster{flying: true}
			destroyer := &fakeDestroyer{}
			req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, first, caster, destroyer, isPet)
			tmpl, _ := req.Inventory.Templates().Get(req.Item.TemplateID)
			tmpl.AttachedSkills = []modelitem.SkillRef{{ID: int32(first.ID), Level: int32(first.Level)}, {ID: int32(second.ID), Level: int32(second.Level)}}
			req.Definitions = fakeDefinitionTable{
				{ID: first.ID, Level: first.Level}:   first,
				{ID: second.ID, Level: second.Level}: second,
			}

			results := UseAll(req)

			if len(results) != 1 || results[0].Outcome != ConditionRejected || results[0].Skill.ID != first.ID {
				t.Fatalf("results = %#v, want first skill condition rejection only", results)
			}
			if results[0].Condition.Root.Kind != "player" {
				t.Fatalf("failed condition = %#v, want the skill's player condition", results[0].Condition)
			}
			if destroyer.calls != 0 || caster.reuseCalls != 0 {
				t.Fatalf("condition failure consumed=%d reuse=%d, want neither", destroyer.calls, caster.reuseCalls)
			}
		})
	}
}

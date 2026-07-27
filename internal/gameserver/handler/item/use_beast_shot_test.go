package item

import (
	"testing"

	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

type fakeBeastShotCharger struct {
	dead      bool
	charged   map[modelitem.ShotKind]bool
	ssCount   int
	spsCount  int
	setCalled map[modelitem.ShotKind]bool
}

func newFakeBeastShotCharger() *fakeBeastShotCharger {
	return &fakeBeastShotCharger{
		charged:   make(map[modelitem.ShotKind]bool),
		setCalled: make(map[modelitem.ShotKind]bool),
	}
}

func (f *fakeBeastShotCharger) Dead() bool { return f.dead }
func (f *fakeBeastShotCharger) ChargedShot(kind modelitem.ShotKind) bool {
	return f.charged[kind]
}
func (f *fakeBeastShotCharger) SetChargedShot(kind modelitem.ShotKind, charged bool) {
	f.setCalled[kind] = charged
	f.charged[kind] = charged
}
func (f *fakeBeastShotCharger) SSCount() int  { return f.ssCount }
func (f *fakeBeastShotCharger) SPSCount() int { return f.spsCount }

func beastShotTemplate(id int32, handler string, skillID int32) *modelitem.Template {
	tmpl := &modelitem.Template{
		ID:      id,
		Kind:    modelitem.KindEtcItem,
		EtcItem: &modelitem.EtcItemDetail{Handler: handler},
	}
	if skillID != 0 {
		tmpl.AttachedSkills = []modelitem.SkillRef{{ID: skillID, Level: 1}}
	}
	return tmpl
}

func TestUseBeastShotSoulshotApplied(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 2033)
	inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{tmpl}))
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 6645}
	summon := newFakeBeastShotCharger()
	summon.ssCount = 5
	destroyer := &fakeDestroyer{}

	res := UseBeastShot(BeastShotUseRequest{Summon: summon, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer})

	if res.Outcome != BeastShotApplied {
		t.Fatalf("Outcome = %v, want BeastShotApplied", res.Outcome)
	}
	if res.SkillID != 2033 {
		t.Fatalf("SkillID = %d, want 2033", res.SkillID)
	}
	if destroyer.calls != 1 {
		t.Fatalf("DestroyItem calls = %d, want 1", destroyer.calls)
	}
	if !summon.setCalled[modelitem.ShotSoul] {
		t.Fatal("SetChargedShot(ShotSoul, true) not called")
	}
}

func TestUseBeastShotSpiritshotAndBlessedResolveDistinctKinds(t *testing.T) {
	destroyer := &fakeDestroyer{}

	spiritTmpl := beastShotTemplate(6646, BeastSpiritShotsHandler, 0)
	inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{spiritTmpl}))
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 6646}
	spiritSummon := newFakeBeastShotCharger()
	if res := UseBeastShot(BeastShotUseRequest{Summon: spiritSummon, Inventory: inv, Item: inst, Template: spiritTmpl, Destroyer: destroyer}); res.Outcome != BeastShotApplied {
		t.Fatalf("spirit Outcome = %v, want BeastShotApplied", res.Outcome)
	}
	if !spiritSummon.setCalled[modelitem.ShotSpirit] {
		t.Fatal("expected ShotSpirit charged")
	}

	blessedTmpl := beastShotTemplate(6647, BeastSpiritShotsHandler, 0)
	blessedSummon := newFakeBeastShotCharger()
	if res := UseBeastShot(BeastShotUseRequest{Summon: blessedSummon, Inventory: inv, Item: inst, Template: blessedTmpl, Destroyer: destroyer}); res.Outcome != BeastShotApplied {
		t.Fatalf("blessed Outcome = %v, want BeastShotApplied", res.Outcome)
	}
	if !blessedSummon.setCalled[modelitem.ShotBlessedSpirit] {
		t.Fatal("expected ShotBlessedSpirit charged")
	}
}

func TestUseBeastShotAlreadyChargedDoesNotConsume(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 0)
	inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{tmpl}))
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 6645}
	summon := newFakeBeastShotCharger()
	summon.charged[modelitem.ShotSoul] = true
	destroyer := &fakeDestroyer{}

	res := UseBeastShot(BeastShotUseRequest{Summon: summon, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer})

	if res.Outcome != BeastShotAlreadyCharged {
		t.Fatalf("Outcome = %v, want BeastShotAlreadyCharged", res.Outcome)
	}
	if destroyer.calls != 0 {
		t.Fatalf("DestroyItem calls = %d, want 0", destroyer.calls)
	}
}

func TestUseBeastShotCallerIsSummonRejected(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 0)
	inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{tmpl}))
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 6645}
	destroyer := &fakeDestroyer{}

	res := UseBeastShot(BeastShotUseRequest{CallerIsSummon: true, Summon: newFakeBeastShotCharger(), Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer})

	if res.Outcome != BeastShotCallerIsSummon {
		t.Fatalf("Outcome = %v, want BeastShotCallerIsSummon", res.Outcome)
	}
	if destroyer.calls != 0 {
		t.Fatalf("DestroyItem calls = %d, want 0", destroyer.calls)
	}
}

func TestUseBeastShotNoSummonRejected(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 0)
	inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{tmpl}))
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 6645}
	destroyer := &fakeDestroyer{}

	res := UseBeastShot(BeastShotUseRequest{Summon: nil, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer})

	if res.Outcome != BeastShotNoSummon {
		t.Fatalf("Outcome = %v, want BeastShotNoSummon", res.Outcome)
	}
}

func TestUseBeastShotSummonDeadRejected(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 0)
	inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{tmpl}))
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 6645}
	summon := newFakeBeastShotCharger()
	summon.dead = true
	destroyer := &fakeDestroyer{}

	res := UseBeastShot(BeastShotUseRequest{Summon: summon, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer})

	if res.Outcome != BeastShotSummonDead {
		t.Fatalf("Outcome = %v, want BeastShotSummonDead", res.Outcome)
	}
	if destroyer.calls != 0 {
		t.Fatalf("DestroyItem calls = %d, want 0", destroyer.calls)
	}
}

func TestUseBeastShotNotEnoughItemsWhenDestroyFails(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 0)
	inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{tmpl}))
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 6645}
	summon := newFakeBeastShotCharger()
	destroyer := &fakeDestroyer{fail: true}

	res := UseBeastShot(BeastShotUseRequest{Summon: summon, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer})

	if res.Outcome != BeastShotNotEnoughItems {
		t.Fatalf("Outcome = %v, want BeastShotNotEnoughItems", res.Outcome)
	}
	if summon.setCalled[modelitem.ShotSoul] {
		t.Fatal("SetChargedShot should not be called when destroy fails")
	}
}

type fakeAutoShotChecker struct{ enabled bool }

func (f *fakeAutoShotChecker) AutoSoulShotEnabled(itemID int32) bool { return f.enabled }

func TestUseBeastShotNotEnoughItemsAutoEnabledPropagatesToResult(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 0)
	inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{tmpl}))
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 6645}
	summon := newFakeBeastShotCharger()
	destroyer := &fakeDestroyer{fail: true}

	res := UseBeastShot(BeastShotUseRequest{Caster: &fakeAutoShotChecker{enabled: true}, Summon: summon, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer})

	if !res.AutoEnabled {
		t.Fatal("AutoEnabled = false, want true")
	}
}

func TestUseBeastShotUnrelatedHandlerNotHandled(t *testing.T) {
	tmpl := beastShotTemplate(500, "SomeOtherHandler", 0)
	inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{tmpl}))
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 500}
	destroyer := &fakeDestroyer{}

	res := UseBeastShot(BeastShotUseRequest{Summon: newFakeBeastShotCharger(), Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer})

	if res.Outcome != BeastShotNotHandled {
		t.Fatalf("Outcome = %v, want BeastShotNotHandled", res.Outcome)
	}
}

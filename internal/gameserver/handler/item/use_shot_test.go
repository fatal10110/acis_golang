package item

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

type fakeShotCharger struct {
	soulshotConsume, spiritshotConsume int32
	soulshotResult, spiritshotResult   player.ChargeShotResult
	autoEnabled                        bool

	gotShotCrystal, gotSpiritCrystal modelitem.CrystalType
	gotSpiritKind                    modelitem.ShotKind
	autoEnabledCalledWith            int32
}

func (f *fakeShotCharger) ChargeSoulshot(shotCrystal modelitem.CrystalType, reducedRoll int) (int32, player.ChargeShotResult) {
	f.gotShotCrystal = shotCrystal
	return f.soulshotConsume, f.soulshotResult
}

func (f *fakeShotCharger) ChargeSpiritshot(kind modelitem.ShotKind, shotCrystal modelitem.CrystalType) (int32, player.ChargeShotResult) {
	f.gotSpiritKind = kind
	f.gotSpiritCrystal = shotCrystal
	return f.spiritshotConsume, f.spiritshotResult
}

func (f *fakeShotCharger) AutoSoulShotEnabled(itemID int32) bool {
	f.autoEnabledCalledWith = itemID
	return f.autoEnabled
}

func shotTemplate(handler string, crystal modelitem.CrystalType, skillID int32) *modelitem.Template {
	tmpl := &modelitem.Template{
		ID:      500,
		Kind:    modelitem.KindEtcItem,
		Crystal: crystal,
		EtcItem: &modelitem.EtcItemDetail{Handler: handler},
	}
	if skillID != 0 {
		tmpl.AttachedSkills = []modelitem.SkillRef{{ID: skillID, Level: 1}}
	}
	return tmpl
}

func TestUseShotSoulshotApplied(t *testing.T) {
	tmpl := shotTemplate(SoulShotsHandler, modelitem.CrystalD, 2154)
	table := modelitem.NewTable([]*modelitem.Template{tmpl})
	inv := itemcontainer.NewPlayerInventory(1, table)
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 500}
	caster := &fakeShotCharger{soulshotConsume: 2, soulshotResult: player.ChargeShotOK}
	destroyer := &fakeDestroyer{}

	res := UseShot(ShotUseRequest{Caster: caster, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer})

	if res.Outcome != ShotApplied {
		t.Fatalf("Outcome = %v, want ShotApplied", res.Outcome)
	}
	if res.SkillID != 2154 {
		t.Fatalf("SkillID = %d, want 2154", res.SkillID)
	}
	if destroyer.calls != 1 {
		t.Fatalf("DestroyItem calls = %d, want 1", destroyer.calls)
	}
	if caster.gotShotCrystal != modelitem.CrystalD {
		t.Fatalf("ChargeSoulshot crystal = %v, want CrystalD", caster.gotShotCrystal)
	}
}

func TestUseShotSpiritshotAndBlessedResolveDistinctKinds(t *testing.T) {
	destroyer := &fakeDestroyer{}

	spiritTmpl := shotTemplate(SpiritShotsHandler, modelitem.CrystalC, 0)
	spiritCaster := &fakeShotCharger{spiritshotConsume: 1, spiritshotResult: player.ChargeShotOK}
	inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{spiritTmpl}))
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 500}
	if res := UseShot(ShotUseRequest{Caster: spiritCaster, Inventory: inv, Item: inst, Template: spiritTmpl, Destroyer: destroyer}); res.Outcome != ShotApplied {
		t.Fatalf("spirit Outcome = %v, want ShotApplied", res.Outcome)
	}
	if spiritCaster.gotSpiritKind != modelitem.ShotSpirit {
		t.Fatalf("ChargeSpiritshot kind = %v, want ShotSpirit", spiritCaster.gotSpiritKind)
	}

	blessedTmpl := shotTemplate(BlessedSpiritShotsHandler, modelitem.CrystalC, 0)
	blessedCaster := &fakeShotCharger{spiritshotConsume: 1, spiritshotResult: player.ChargeShotOK}
	if res := UseShot(ShotUseRequest{Caster: blessedCaster, Inventory: inv, Item: inst, Template: blessedTmpl, Destroyer: destroyer}); res.Outcome != ShotApplied {
		t.Fatalf("blessed Outcome = %v, want ShotApplied", res.Outcome)
	}
	if blessedCaster.gotSpiritKind != modelitem.ShotBlessedSpirit {
		t.Fatalf("ChargeSpiritshot kind = %v, want ShotBlessedSpirit", blessedCaster.gotSpiritKind)
	}
}

func TestUseShotRejectionsDoNotConsume(t *testing.T) {
	tests := []struct {
		name         string
		chargeResult player.ChargeShotResult
		wantOutcome  ShotOutcome
	}{
		{"no capacity", player.ChargeShotNoCapacity, ShotNoCapacity},
		{"grade mismatch", player.ChargeShotGradeMismatch, ShotGradeMismatch},
		{"already charged", player.ChargeShotAlreadyCharged, ShotAlreadyCharged},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := shotTemplate(SoulShotsHandler, modelitem.CrystalD, 0)
			inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{tmpl}))
			inst := &modelitem.Instance{ObjectID: 10, TemplateID: 500}
			caster := &fakeShotCharger{soulshotResult: tt.chargeResult}
			destroyer := &fakeDestroyer{}

			res := UseShot(ShotUseRequest{Caster: caster, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer})

			if res.Outcome != tt.wantOutcome {
				t.Fatalf("Outcome = %v, want %v", res.Outcome, tt.wantOutcome)
			}
			if destroyer.calls != 0 {
				t.Fatalf("DestroyItem calls = %d, want 0", destroyer.calls)
			}
		})
	}
}

func TestUseShotNotEnoughItemsWhenDestroyFails(t *testing.T) {
	tmpl := shotTemplate(SoulShotsHandler, modelitem.CrystalD, 0)
	inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{tmpl}))
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 500}
	caster := &fakeShotCharger{soulshotConsume: 1, soulshotResult: player.ChargeShotOK}
	destroyer := &fakeDestroyer{fail: true}

	res := UseShot(ShotUseRequest{Caster: caster, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer})

	if res.Outcome != ShotNotEnoughItems {
		t.Fatalf("Outcome = %v, want ShotNotEnoughItems", res.Outcome)
	}
}

func TestUseShotAutoEnabledPropagatesToResult(t *testing.T) {
	tmpl := shotTemplate(SoulShotsHandler, modelitem.CrystalD, 0)
	inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{tmpl}))
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 500}
	caster := &fakeShotCharger{soulshotResult: player.ChargeShotNoCapacity, autoEnabled: true}
	destroyer := &fakeDestroyer{}

	res := UseShot(ShotUseRequest{Caster: caster, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer})

	if !res.AutoEnabled {
		t.Fatal("AutoEnabled = false, want true")
	}
	if caster.autoEnabledCalledWith != tmpl.ID {
		t.Fatalf("AutoSoulShotEnabled called with %d, want template id %d", caster.autoEnabledCalledWith, tmpl.ID)
	}
}

func TestUseShotUnrelatedHandlerNotHandled(t *testing.T) {
	tmpl := shotTemplate("SomeOtherHandler", modelitem.CrystalD, 0)
	inv := itemcontainer.NewPlayerInventory(1, modelitem.NewTable([]*modelitem.Template{tmpl}))
	inst := &modelitem.Instance{ObjectID: 10, TemplateID: 500}
	caster := &fakeShotCharger{}
	destroyer := &fakeDestroyer{}

	res := UseShot(ShotUseRequest{Caster: caster, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer})

	if res.Outcome != ShotNotHandled {
		t.Fatalf("Outcome = %v, want ShotNotHandled", res.Outcome)
	}
}

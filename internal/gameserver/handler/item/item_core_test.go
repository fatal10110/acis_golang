package item

import (
	"errors"
	"testing"
	"time"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ---- from cast_ai_test.go ----
func newCastAICharacter(id int32) *player.Character {
	ch := &player.Character{ID: id}
	ch.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 100, CurrentMP: 100})
	return ch
}

func startedCastAIController(t *testing.T, caster *player.Character, def modelskill.Definition) *actorcast.Controller {
	t.Helper()
	ctrl := actorcast.NewController(actorcast.PlayerActor{Character: caster})
	if _, err := ctrl.Start(time.Unix(1000, 0), caster, def); err != nil {
		t.Fatalf("setup Start() error: %v", err)
	}
	return ctrl
}

func TestConsumeAICastItemLeavesControllerCastingOnSuccess(t *testing.T) {
	def := modelskill.Definition{ID: 9, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf}
	caster := newCastAICharacter(10)
	ctrl := startedCastAIController(t, caster, def)

	inv := itemcontainer.NewPlayerInventory(caster.ID, modelitem.NewTable(nil))
	inst := &modelitem.Instance{ObjectID: 20, TemplateID: 1}
	destroyer := &fakeDestroyer{}

	if consumed := ConsumeAICastItem(ConsumeAICastItemRequest{
		Controller: ctrl,
		Definition: def,
		Inventory:  inv,
		Item:       inst,
		Destroyer:  destroyer,
	}); consumed.Err != nil {
		t.Fatalf("ConsumeAICastItem() error: %v", consumed.Err)
	}
	if destroyer.calls != 1 {
		t.Fatalf("DestroyItem calls = %d, want 1", destroyer.calls)
	}
	// The caller drives the cast's Launch/Hit/Finish phases through
	// Controller.Schedule after consuming the item (network/item_skill_cast.go);
	// consuming must not itself clear the cast.
	if !ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = false after a successful consume, want still casting")
	}
}

func TestConsumeAICastItemStopsControllerWhenItemMissing(t *testing.T) {
	def := modelskill.Definition{ID: 9, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf}
	caster := newCastAICharacter(10)
	ctrl := startedCastAIController(t, caster, def)

	inv := itemcontainer.NewPlayerInventory(caster.ID, modelitem.NewTable(nil))
	inst := &modelitem.Instance{ObjectID: 20, TemplateID: 1}
	destroyer := &fakeDestroyer{fail: true}

	consumed := ConsumeAICastItem(ConsumeAICastItemRequest{
		Controller: ctrl,
		Definition: def,
		Inventory:  inv,
		Item:       inst,
		Destroyer:  destroyer,
	})

	if !errors.Is(consumed.Err, actorcast.ErrNotEnoughItems) {
		t.Fatalf("ConsumeAICastItem() error = %v, want ErrNotEnoughItems", consumed.Err)
	}
	if ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = true after a rejection, want stopped/cleared")
	}
}

func TestConsumeAICastItemReportsSharedReuseGroup(t *testing.T) {
	def := modelskill.Definition{ID: 9, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, ReuseDelay: 5000}
	inv := itemcontainer.NewPlayerInventory(10, modelitem.NewTable(nil))
	inst := &modelitem.Instance{ObjectID: 20, TemplateID: 1}
	destroyer := &fakeDestroyer{}

	t.Run("no group defined", func(t *testing.T) {
		caster := newCastAICharacter(10)
		ctrl := startedCastAIController(t, caster, def)
		tmpl := &modelitem.Template{ID: 1, EtcItem: &modelitem.EtcItemDetail{SharedReuseGroup: -1}}
		res := ConsumeAICastItem(ConsumeAICastItemRequest{
			Controller: ctrl, Definition: def, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer,
		})
		if res.Err != nil {
			t.Fatalf("ConsumeAICastItem() error: %v", res.Err)
		}
		if res.SharedReuseGroup != -1 {
			t.Fatalf("SharedReuseGroup = %d, want -1", res.SharedReuseGroup)
		}
	})

	t.Run("group defined, item reuse longer than skill's", func(t *testing.T) {
		caster := newCastAICharacter(11)
		ctrl := startedCastAIController(t, caster, def)
		tmpl := &modelitem.Template{ID: 1, EtcItem: &modelitem.EtcItemDetail{SharedReuseGroup: 3, ReuseDelay: 8000}}
		res := ConsumeAICastItem(ConsumeAICastItemRequest{
			Controller: ctrl, Definition: def, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer,
		})
		if res.Err != nil {
			t.Fatalf("ConsumeAICastItem() error: %v", res.Err)
		}
		if res.SharedReuseGroup != 3 {
			t.Fatalf("SharedReuseGroup = %d, want 3", res.SharedReuseGroup)
		}
		if res.ReuseMillis != 8000 {
			t.Fatalf("ReuseMillis = %d, want 8000 (item's reuse, longer than the skill's 5000)", res.ReuseMillis)
		}
	})
}

// ---- from karma_teleport_test.go ----
func TestIsTeleportOrRecallSkillType(t *testing.T) {
	tests := []struct {
		skillType string
		want      bool
	}{
		{"TELEPORT", true},
		{"RECALL", true},
		{"BUFF", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isTeleportOrRecallSkillType(tt.skillType); got != tt.want {
			t.Errorf("isTeleportOrRecallSkillType(%q) = %v, want %v", tt.skillType, got, tt.want)
		}
	}
}

func TestIsRecallSkillType(t *testing.T) {
	tests := []struct {
		skillType string
		want      bool
	}{
		{"RECALL", true},
		{"TELEPORT", false},
		{"BUFF", false},
	}
	for _, tt := range tests {
		if got := isRecallSkillType(tt.skillType); got != tt.want {
			t.Errorf("isRecallSkillType(%q) = %v, want %v", tt.skillType, got, tt.want)
		}
	}
}

func TestItemBlockedByKarmaTeleport(t *testing.T) {
	recall := modelskill.Definition{ID: 1050, Level: 1, SkillType: "RECALL"}
	buff := modelskill.Definition{ID: 2005, Level: 1, SkillType: "BUFF"}

	recallTmpl := &modelitem.Template{AttachedSkills: []modelitem.SkillRef{{ID: 1050, Level: 1}}}
	buffTmpl := &modelitem.Template{AttachedSkills: []modelitem.SkillRef{{ID: 2005, Level: 1}}}
	unresolvedTmpl := &modelitem.Template{AttachedSkills: []modelitem.SkillRef{{ID: 9999, Level: 1}}}

	tests := []struct {
		name                   string
		tmpl                   *modelitem.Template
		defs                   aiCastDefinitions
		karma                  int
		karmaPlayerCanTeleport bool
		want                   bool
	}{
		{"nil template", nil, aiCastDefinitions{}, 1, false, false},
		{"karma zero does not block", recallTmpl, aiCastDefinitions{{ID: 1050, Level: 1}: recall}, 0, false, false},
		{"karma positive but config allows teleport", recallTmpl, aiCastDefinitions{{ID: 1050, Level: 1}: recall}, 1, true, false},
		{"karma positive blocks recall item", recallTmpl, aiCastDefinitions{{ID: 1050, Level: 1}: recall}, 1, false, true},
		{"karma positive does not block non-teleport skill", buffTmpl, aiCastDefinitions{{ID: 2005, Level: 1}: buff}, 1, false, false},
		{"unresolved attached skill does not block", unresolvedTmpl, aiCastDefinitions{}, 1, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ItemBlockedByKarmaTeleport(tt.tmpl, tt.defs, tt.karma, tt.karmaPlayerCanTeleport); got != tt.want {
				t.Errorf("ItemBlockedByKarmaTeleport() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecallCastBlockedByKarma(t *testing.T) {
	tests := []struct {
		name                   string
		skillType              string
		karma                  int
		karmaPlayerCanTeleport bool
		want                   bool
	}{
		{"recall blocked with positive karma", "RECALL", 1, false, true},
		{"recall allowed by config", "RECALL", 1, true, false},
		{"recall allowed with zero karma", "RECALL", 0, false, false},
		{"teleport type is not gated by direct cast", "TELEPORT", 1, false, false},
		{"non-recall skill not blocked", "BUFF", 1, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RecallCastBlockedByKarma(tt.skillType, tt.karma, tt.karmaPlayerCanTeleport); got != tt.want {
				t.Errorf("RecallCastBlockedByKarma() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---- from use_beast_shot_test.go ----
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

// ---- from use_shot_test.go ----
type fakeShotCharger struct {
	soulshotConsume, spiritshotConsume int32
	soulshotResult, spiritshotResult   player.ChargeShotResult
	autoEnabled                        bool

	gotShotCrystal, gotSpiritCrystal modelitem.CrystalType
	gotSpiritKind                    modelitem.ShotKind
	autoEnabledCalledWith            int32
	setChargedShotCalls              int
	setChargedShotKind               modelitem.ShotKind
	setChargedShotValue              bool
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

func (f *fakeShotCharger) SetChargedShot(kind modelitem.ShotKind, charged bool) {
	f.setChargedShotCalls++
	f.setChargedShotKind = kind
	f.setChargedShotValue = charged
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
	if caster.setChargedShotCalls != 1 || caster.setChargedShotKind != modelitem.ShotSoul || !caster.setChargedShotValue {
		t.Fatalf("SetChargedShot calls = %d kind = %v value = %v, want 1/ShotSoul/true", caster.setChargedShotCalls, caster.setChargedShotKind, caster.setChargedShotValue)
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
	if caster.setChargedShotCalls != 0 {
		t.Fatalf("SetChargedShot calls = %d, want 0: a failed destroy must not leave the weapon charged (SoulShots.java:49-62)", caster.setChargedShotCalls)
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

// ---- from use_skill_ai_cast_test.go ----
type aiCastDefinitions map[modelskill.Ref]modelskill.Definition

func (d aiCastDefinitions) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	def, ok := d[ref]
	return def, ok
}

func TestResolveAICastSkills(t *testing.T) {
	scroll := modelskill.Definition{ID: 2005, Level: 1, Activation: modelskill.ActivationActive}
	other := modelskill.Definition{ID: 2006, Level: 1, Activation: modelskill.ActivationActive}
	potion := modelskill.Definition{ID: 2031, Level: 1, Potion: true}

	tests := []struct {
		name    string
		tmpl    *modelitem.Template
		defs    actorcast.Definitions
		wantIDs []modelskill.ID
	}{
		{
			name: "non-potion carried skill resolves",
			tmpl: &modelitem.Template{
				Kind:           modelitem.KindEtcItem,
				EtcItem:        &modelitem.EtcItemDetail{Handler: ItemSkillsHandler},
				AttachedSkills: []modelitem.SkillRef{{ID: 2005, Level: 1}},
			},
			defs:    aiCastDefinitions{{ID: 2005, Level: 1}: scroll},
			wantIDs: []modelskill.ID{2005},
		},
		{
			name: "later non-potion skills are kept in template order",
			tmpl: &modelitem.Template{
				Kind:           modelitem.KindEtcItem,
				EtcItem:        &modelitem.EtcItemDetail{Handler: ItemSkillsHandler},
				AttachedSkills: []modelitem.SkillRef{{ID: 2005, Level: 1}, {ID: 2006, Level: 1}},
			},
			defs:    aiCastDefinitions{{ID: 2005, Level: 1}: scroll, {ID: 2006, Level: 1}: other},
			wantIDs: []modelskill.ID{2005, 2006},
		},
		{
			name: "potion carried skill is left to the instant-cast path",
			tmpl: &modelitem.Template{
				Kind:           modelitem.KindEtcItem,
				EtcItem:        &modelitem.EtcItemDetail{Handler: ItemSkillsHandler},
				AttachedSkills: []modelitem.SkillRef{{ID: 2031, Level: 1}},
			},
			defs: aiCastDefinitions{{ID: 2031, Level: 1}: potion},
		},
		{
			name: "non-ItemSkills handler is not handled",
			tmpl: &modelitem.Template{
				Kind:           modelitem.KindEtcItem,
				EtcItem:        &modelitem.EtcItemDetail{Handler: "SomeOtherHandler"},
				AttachedSkills: []modelitem.SkillRef{{ID: 2005, Level: 1}},
			},
			defs: aiCastDefinitions{{ID: 2005, Level: 1}: scroll},
		},
		{
			name: "no attached skills is not handled",
			tmpl: &modelitem.Template{
				Kind:    modelitem.KindEtcItem,
				EtcItem: &modelitem.EtcItemDetail{Handler: ItemSkillsHandler},
			},
			defs: aiCastDefinitions{},
		},
		{
			name: "nil template is not handled",
			tmpl: nil,
			defs: aiCastDefinitions{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveAICastSkills(tt.tmpl, tt.defs)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.wantIDs))
			}
			for i, id := range tt.wantIDs {
				if got[i].ID != id {
					t.Fatalf("skill %d ID = %v, want %v", i, got[i].ID, id)
				}
			}
		})
	}
}

// ---- from use_skill_shortbuff_test.go ----
func TestUseDrivesShortBuffForHPPotionFamily(t *testing.T) {
	potion := modelskill.Definition{
		ID: 2031, Level: 1, Potion: true,
		Effects: []modelskill.EffectTemplate{{Count: 7, Time: 2}}, // 14s
	}
	caster := &fakeCaster{}
	destroyer := &fakeDestroyer{}
	req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, destroyer, false)

	res := Use(req)

	if res.Outcome != Applied {
		t.Fatalf("Outcome = %v, want Applied", res.Outcome)
	}
	if !res.HasShortBuff {
		t.Fatal("HasShortBuff = false, want true for an HP-potion-family skill")
	}
	if res.ShortBuffSkillID != 2031 || res.ShortBuffLevel != 1 || res.ShortBuffDurationSeconds != 14 {
		t.Fatalf("short buff = skill %d level %d duration %d, want 2031/1/14", res.ShortBuffSkillID, res.ShortBuffLevel, res.ShortBuffDurationSeconds)
	}
}

func TestUseSkipsShortBuffForNonHPPotionSkill(t *testing.T) {
	potion := modelskill.Definition{
		ID: 9999, Level: 1, Potion: true,
		Effects: []modelskill.EffectTemplate{{Count: 7, Time: 2}},
	}
	caster := &fakeCaster{}
	destroyer := &fakeDestroyer{}
	req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, destroyer, false)

	res := Use(req)

	if res.HasShortBuff {
		t.Fatal("HasShortBuff = true, want false for a skill outside the HP-potion family")
	}
}

func TestUseSkipsShortBuffWhenIDLosesToCurrent(t *testing.T) {
	// A Lesser Healing Potion (2031) must not override a Greater Healing
	// Potion (2037) already showing on the HUD, matching the reference's
	// own id-ordering gate.
	potion := modelskill.Definition{
		ID: 2031, Level: 1, Potion: true,
		Effects: []modelskill.EffectTemplate{{Count: 7, Time: 2}},
	}
	caster := &fakeCaster{shortBuffTaskSkillID: 2037}
	destroyer := &fakeDestroyer{}
	req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, destroyer, false)

	res := Use(req)

	if res.HasShortBuff {
		t.Fatal("HasShortBuff = true, want false when the new skill id loses to the currently-showing one")
	}
}

func TestUseAllowsShortBuffWhenIDMatchesOrWins(t *testing.T) {
	potion := modelskill.Definition{
		ID: 2037, Level: 1, Potion: true,
		Effects: []modelskill.EffectTemplate{{Count: 7, Time: 2}},
	}
	caster := &fakeCaster{shortBuffTaskSkillID: 2031}
	destroyer := &fakeDestroyer{}
	req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, destroyer, false)

	res := Use(req)

	if !res.HasShortBuff {
		t.Fatal("HasShortBuff = false, want true when the new skill id is numerically >= the currently-showing one")
	}
}

// ---- from use_skill_summon_test.go ----
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

// ---- from use_skill_test.go ----
type fakeCaster struct {
	disabled             map[int32]bool
	disableCalls         int
	reuseCalls           int
	shortBuffTaskSkillID int32
	flying               bool
}

func (f *fakeCaster) ObjectID() int32                { return 1 }
func (f *fakeCaster) Position() (int, int, int)      { return 0, 0, 0 }
func (f *fakeCaster) Heading() int                   { return 0 }
func (f *fakeCaster) Dead() bool                     { return false }
func (f *fakeCaster) Category() skilltarget.Category { return skilltarget.CategoryPlayable }
func (f *fakeCaster) SkillDisabled(key int32) bool   { return f.disabled != nil && f.disabled[key] }
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

package item

import (
	"errors"
	"testing"
	"time"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// ---- from cast_ai_test.go ----
func aiCastItemTemplate() *modelitem.Template {
	return &modelitem.Template{ID: 1, Kind: modelitem.KindEtcItem, Stackable: true, Destroyable: true, EtcItem: &modelitem.EtcItemDetail{SharedReuseGroup: -1, ReuseDelay: 8000}}
}

func TestConsumeAICastItemInstallsItemReuse(t *testing.T) {
	def := modelskill.Definition{ID: 9, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, ReuseDelay: 5000}
	tmpl := aiCastItemTemplate()
	caster := newPlayer(10, []*modelitem.Template{tmpl}, carried(20, tmpl.ID, 1))
	inv := caster.Inventory()

	consumed := ConsumeAICastItem(ConsumeAICastItemRequest{
		Caster: caster, Definition: def, Inventory: inv, Item: inv.ItemByObjectID(20), Template: tmpl, Destroyer: destroyer(),
	})
	if consumed.Err != nil {
		t.Fatalf("ConsumeAICastItem() error: %v", consumed.Err)
	}
	if got := stackCount(inv, 20); got != 0 {
		t.Fatalf("item stack after consume = %d, want 0 (the one carried unit destroyed)", got)
	}
	if !caster.SkillDisabled(actorcast.ReuseKey(def)) {
		t.Fatal("skill not disabled after the item was consumed, want the item reuse installed")
	}
}

func TestConsumeAICastItemMissingItemChangesNothing(t *testing.T) {
	def := modelskill.Definition{ID: 9, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, ReuseDelay: 5000}
	tmpl := aiCastItemTemplate()
	caster := newPlayer(10, []*modelitem.Template{tmpl}, carried(21, tmpl.ID, 1))
	inv := caster.Inventory()

	// Object 20 is not carried, so there is nothing to destroy.
	consumed := ConsumeAICastItem(ConsumeAICastItemRequest{
		Caster: caster, Definition: def, Inventory: inv, Item: carried(20, tmpl.ID, 1), Destroyer: destroyer(),
	})
	if !errors.Is(consumed.Err, actorcast.ErrNotEnoughItems) {
		t.Fatalf("ConsumeAICastItem() error = %v, want ErrNotEnoughItems", consumed.Err)
	}
	if caster.SkillDisabled(actorcast.ReuseKey(def)) {
		t.Fatal("skill disabled after a failed consume, want no reuse")
	}
	if got := stackCount(inv, 21); got != 1 {
		t.Fatalf("unrelated stack = %d, want untouched 1", got)
	}
}

func TestConsumeAICastItemReportsSharedReuseGroup(t *testing.T) {
	def := modelskill.Definition{ID: 9, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, ReuseDelay: 5000}
	carriedTmpl := aiCastItemTemplate()

	t.Run("no group defined", func(t *testing.T) {
		caster := newPlayer(10, []*modelitem.Template{carriedTmpl}, carried(20, carriedTmpl.ID, 1))
		inv := caster.Inventory()
		tmpl := &modelitem.Template{ID: 1, EtcItem: &modelitem.EtcItemDetail{SharedReuseGroup: -1}}
		res := ConsumeAICastItem(ConsumeAICastItemRequest{
			Caster: caster, Definition: def, Inventory: inv, Item: inv.ItemByObjectID(20), Template: tmpl, Destroyer: destroyer(),
		})
		if res.Err != nil {
			t.Fatalf("ConsumeAICastItem() error: %v", res.Err)
		}
		if res.SharedReuseGroup != -1 {
			t.Fatalf("SharedReuseGroup = %d, want -1", res.SharedReuseGroup)
		}
	})

	t.Run("group defined, item reuse longer than skill's", func(t *testing.T) {
		caster := newPlayer(11, []*modelitem.Template{carriedTmpl}, carried(20, carriedTmpl.ID, 1))
		inv := caster.Inventory()
		tmpl := &modelitem.Template{ID: 1, EtcItem: &modelitem.EtcItemDetail{SharedReuseGroup: 3, ReuseDelay: 8000}}
		res := ConsumeAICastItem(ConsumeAICastItemRequest{
			Caster: caster, Definition: def, Inventory: inv, Item: inv.ItemByObjectID(20), Template: tmpl, Destroyer: destroyer(),
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
		defs                   actorcast.Definitions
		karma                  int
		karmaPlayerCanTeleport bool
		want                   bool
	}{
		{"nil template", nil, definitions(), 1, false, false},
		{"karma zero does not block", recallTmpl, definitions(recall), 0, false, false},
		{"karma positive but config allows teleport", recallTmpl, definitions(recall), 1, true, false},
		{"karma positive blocks recall item", recallTmpl, definitions(recall), 1, false, true},
		{"karma positive does not block non-teleport skill", buffTmpl, definitions(buff), 1, false, false},
		{"unresolved attached skill does not block", unresolvedTmpl, definitions(), 1, false, false},
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
func beastShotTemplate(id int32, handler string, skillID int32) *modelitem.Template {
	tmpl := &modelitem.Template{
		ID:          id,
		Kind:        modelitem.KindEtcItem,
		Stackable:   true,
		Destroyable: true,
		EtcItem:     &modelitem.EtcItemDetail{Handler: handler},
	}
	if skillID != 0 {
		tmpl.AttachedSkills = []modelitem.SkillRef{{ID: skillID, Level: 1}}
	}
	return tmpl
}

// beastShotStack returns an inventory carrying count units of tmpl as
// object 10.
func beastShotStack(tmpl *modelitem.Template, count int) (*itemcontainer.Inventory, *modelitem.Instance) {
	inv := itemcontainer.RestorePlayerInventory(1, modelitem.NewTable([]*modelitem.Template{tmpl}), []*modelitem.Instance{carried(10, tmpl.ID, count)})
	return inv, inv.ItemByObjectID(10)
}

var beastShotKinds = []modelitem.ShotKind{modelitem.ShotSoul, modelitem.ShotSpirit, modelitem.ShotBlessedSpirit}

// chargedKinds lists the shot kinds charged on s.
func chargedKinds(s BeastShotCharger) []modelitem.ShotKind {
	var kinds []modelitem.ShotKind
	for _, kind := range beastShotKinds {
		if s.ChargedShot(kind) {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

func TestUseBeastShotSoulshotApplied(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 2033)
	inv, inst := beastShotStack(tmpl, 12)
	servitor := newServitor(t, 2, 5, 3)

	res := UseBeastShot(BeastShotUseRequest{Summon: servitor, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})

	if res.Outcome != BeastShotApplied {
		t.Fatalf("Outcome = %v, want BeastShotApplied", res.Outcome)
	}
	if res.SkillID != 2033 {
		t.Fatalf("SkillID = %d, want 2033", res.SkillID)
	}
	if got := stackCount(inv, 10); got != 7 {
		t.Fatalf("stack left = %d, want 7 (12 minus the servitor's 5 soulshots per charge)", got)
	}
	if got := chargedKinds(servitor); len(got) != 1 || got[0] != modelitem.ShotSoul {
		t.Fatalf("servitor charged kinds = %v, want [soulshot]", got)
	}
}

func TestUseBeastShotSpiritshotAndBlessedResolveDistinctKinds(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   int32
		want modelitem.ShotKind
	}{
		{"spiritshot", 6646, modelitem.ShotSpirit},
		{"blessed spiritshot", 6647, modelitem.ShotBlessedSpirit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := beastShotTemplate(tc.id, BeastSpiritShotsHandler, 0)
			inv, inst := beastShotStack(tmpl, 12)
			servitor := newServitor(t, 2, 5, 3)

			if res := UseBeastShot(BeastShotUseRequest{Summon: servitor, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()}); res.Outcome != BeastShotApplied {
				t.Fatalf("Outcome = %v, want BeastShotApplied", res.Outcome)
			}
			if got := chargedKinds(servitor); len(got) != 1 || got[0] != tc.want {
				t.Fatalf("servitor charged kinds = %v, want [%v]", got, tc.want)
			}
			if got := stackCount(inv, 10); got != 9 {
				t.Fatalf("stack left = %d, want 9 (12 minus the servitor's 3 spiritshots per charge)", got)
			}
		})
	}
}

func TestUseBeastShotAlreadyChargedDoesNotConsume(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 0)
	inv, inst := beastShotStack(tmpl, 12)
	servitor := newServitor(t, 2, 5, 3)
	servitor.SetChargedShot(modelitem.ShotSoul, true)

	res := UseBeastShot(BeastShotUseRequest{Summon: servitor, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})

	if res.Outcome != BeastShotAlreadyCharged {
		t.Fatalf("Outcome = %v, want BeastShotAlreadyCharged", res.Outcome)
	}
	if got := stackCount(inv, 10); got != 12 {
		t.Fatalf("stack left = %d, want untouched 12", got)
	}
}

func TestUseBeastShotCallerIsSummonRejected(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 0)
	inv, inst := beastShotStack(tmpl, 12)
	servitor := newServitor(t, 2, 5, 3)

	res := UseBeastShot(BeastShotUseRequest{CallerIsSummon: true, Summon: servitor, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})

	if res.Outcome != BeastShotCallerIsSummon {
		t.Fatalf("Outcome = %v, want BeastShotCallerIsSummon", res.Outcome)
	}
	if got := stackCount(inv, 10); got != 12 {
		t.Fatalf("stack left = %d, want untouched 12", got)
	}
	if got := chargedKinds(servitor); len(got) != 0 {
		t.Fatalf("servitor charged kinds = %v, want none", got)
	}
}

func TestUseBeastShotNoSummonRejected(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 0)
	inv, inst := beastShotStack(tmpl, 12)

	res := UseBeastShot(BeastShotUseRequest{Summon: nil, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})

	if res.Outcome != BeastShotNoSummon {
		t.Fatalf("Outcome = %v, want BeastShotNoSummon", res.Outcome)
	}
	if got := stackCount(inv, 10); got != 12 {
		t.Fatalf("stack left = %d, want untouched 12", got)
	}
}

func TestUseBeastShotSummonDeadRejected(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 0)
	inv, inst := beastShotStack(tmpl, 12)
	servitor := newServitor(t, 2, 5, 3)
	if !servitor.Kill(nil) {
		t.Fatal("Kill() = false, want the servitor dead")
	}

	res := UseBeastShot(BeastShotUseRequest{Summon: servitor, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})

	if res.Outcome != BeastShotSummonDead {
		t.Fatalf("Outcome = %v, want BeastShotSummonDead", res.Outcome)
	}
	if got := stackCount(inv, 10); got != 12 {
		t.Fatalf("stack left = %d, want untouched 12", got)
	}
}

func TestUseBeastShotNotEnoughItemsWhenDestroyFails(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 0)
	inv, inst := beastShotStack(tmpl, 4) // the servitor needs 5
	servitor := newServitor(t, 2, 5, 3)

	res := UseBeastShot(BeastShotUseRequest{Summon: servitor, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})

	if res.Outcome != BeastShotNotEnoughItems {
		t.Fatalf("Outcome = %v, want BeastShotNotEnoughItems", res.Outcome)
	}
	if res.AutoEnabled {
		t.Fatal("AutoEnabled = true with no caster, want false")
	}
	if got := chargedKinds(servitor); len(got) != 0 {
		t.Fatalf("servitor charged kinds = %v, want none: a failed destroy must not charge", got)
	}
	if got := stackCount(inv, 10); got != 4 {
		t.Fatalf("stack left = %d, want untouched 4", got)
	}
}

func TestUseBeastShotNotEnoughItemsAutoEnabledPropagatesToResult(t *testing.T) {
	tmpl := beastShotTemplate(6645, BeastSoulShotsHandler, 0)
	for _, tc := range []struct {
		name     string
		autoID   int32
		wantAuto bool
	}{
		{"this shot on auto", tmpl.ID, true},
		{"another shot on auto", 6646, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv, inst := beastShotStack(tmpl, 4)
			owner := &player.Character{ID: 1}
			owner.SetAutoSoulShot(tc.autoID, true)

			res := UseBeastShot(BeastShotUseRequest{Caster: owner, Summon: newServitor(t, 2, 5, 3), Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})

			if res.Outcome != BeastShotNotEnoughItems {
				t.Fatalf("Outcome = %v, want BeastShotNotEnoughItems", res.Outcome)
			}
			if res.AutoEnabled != tc.wantAuto {
				t.Fatalf("AutoEnabled = %v, want %v", res.AutoEnabled, tc.wantAuto)
			}
		})
	}
}

func TestUseBeastShotUnrelatedHandlerNotHandled(t *testing.T) {
	tmpl := beastShotTemplate(500, "SomeOtherHandler", 0)
	inv, inst := beastShotStack(tmpl, 12)
	servitor := newServitor(t, 2, 5, 3)

	res := UseBeastShot(BeastShotUseRequest{Summon: servitor, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})

	if res.Outcome != BeastShotNotHandled {
		t.Fatalf("Outcome = %v, want BeastShotNotHandled", res.Outcome)
	}
	if got := stackCount(inv, 10); got != 12 {
		t.Fatalf("stack left = %d, want untouched 12", got)
	}
}

// ---- from use_shot_test.go ----
func shotTemplate(handler string, crystal modelitem.CrystalType, skillID int32) *modelitem.Template {
	tmpl := &modelitem.Template{
		ID:          500,
		Kind:        modelitem.KindEtcItem,
		Crystal:     crystal,
		Stackable:   true,
		Destroyable: true,
		EtcItem:     &modelitem.EtcItemDetail{Handler: handler},
	}
	if skillID != 0 {
		tmpl.AttachedSkills = []modelitem.SkillRef{{ID: skillID, Level: 1}}
	}
	return tmpl
}

// shotWeaponID is a sword taking 2 soulshots or 1 spiritshot per charge.
const shotWeaponID = 600

func shotWeapon(crystal modelitem.CrystalType) *modelitem.Template {
	return &modelitem.Template{
		ID: shotWeaponID, Kind: modelitem.KindWeapon, Slot: modelitem.SlotRHand, Crystal: crystal,
		Weapon: &modelitem.WeaponDetail{Type: modelitem.WeaponSword, SoulshotCount: 2, SpiritshotCount: 1},
	}
}

// newShooter returns a player carrying count units of shot as object 10
// and, when weapon is set, wielding it as object 20.
func newShooter(shot, weapon *modelitem.Template, count int) (*player.Character, *itemcontainer.Inventory, *modelitem.Instance) {
	templates := []*modelitem.Template{shot}
	items := []*modelitem.Instance{carried(10, shot.ID, count)}
	if weapon != nil {
		templates = append(templates, weapon)
		items = append(items, equipped(20, weapon.ID))
	}
	ch := newPlayer(1, templates, items...)
	inv := ch.Inventory()
	return ch, inv, inv.ItemByObjectID(10)
}

// weaponCharges lists the shot kinds charged on ch's weapon.
func weaponCharges(ch *player.Character) []modelitem.ShotKind {
	var kinds []modelitem.ShotKind
	for _, kind := range beastShotKinds {
		if ch.ChargedShot(kind) {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

func TestUseShotSoulshotApplied(t *testing.T) {
	tmpl := shotTemplate(SoulShotsHandler, modelitem.CrystalD, 2154)
	caster, inv, inst := newShooter(tmpl, shotWeapon(modelitem.CrystalD), 10)

	res := UseShot(ShotUseRequest{Caster: caster, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})

	if res.Outcome != ShotApplied {
		t.Fatalf("Outcome = %v, want ShotApplied", res.Outcome)
	}
	if res.SkillID != 2154 {
		t.Fatalf("SkillID = %d, want 2154", res.SkillID)
	}
	if got := stackCount(inv, 10); got != 8 {
		t.Fatalf("stack left = %d, want 8 (10 minus the weapon's 2 soulshots)", got)
	}
	if got := weaponCharges(caster); len(got) != 1 || got[0] != modelitem.ShotSoul {
		t.Fatalf("weapon charges = %v, want [soulshot]", got)
	}
}

func TestUseShotSpiritshotAndBlessedResolveDistinctKinds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler string
		want    modelitem.ShotKind
	}{
		{"spiritshot", SpiritShotsHandler, modelitem.ShotSpirit},
		{"blessed spiritshot", BlessedSpiritShotsHandler, modelitem.ShotBlessedSpirit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := shotTemplate(tc.handler, modelitem.CrystalC, 0)
			caster, inv, inst := newShooter(tmpl, shotWeapon(modelitem.CrystalC), 10)

			if res := UseShot(ShotUseRequest{Caster: caster, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()}); res.Outcome != ShotApplied {
				t.Fatalf("Outcome = %v, want ShotApplied", res.Outcome)
			}
			if got := weaponCharges(caster); len(got) != 1 || got[0] != tc.want {
				t.Fatalf("weapon charges = %v, want [%v]", got, tc.want)
			}
			if got := stackCount(inv, 10); got != 9 {
				t.Fatalf("stack left = %d, want 9 (10 minus the weapon's 1 spiritshot)", got)
			}
		})
	}
}

func TestUseShotRejectionsDoNotConsume(t *testing.T) {
	tests := []struct {
		name        string
		weapon      *modelitem.Template
		precharged  bool
		wantOutcome ShotOutcome
	}{
		{"no capacity", nil, false, ShotNoCapacity},
		{"grade mismatch", shotWeapon(modelitem.CrystalC), false, ShotGradeMismatch},
		{"already charged", shotWeapon(modelitem.CrystalD), true, ShotAlreadyCharged},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := shotTemplate(SoulShotsHandler, modelitem.CrystalD, 0)
			caster, inv, inst := newShooter(tmpl, tt.weapon, 10)
			if tt.precharged {
				caster.SetChargedShot(modelitem.ShotSoul, true)
			}

			res := UseShot(ShotUseRequest{Caster: caster, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})

			if res.Outcome != tt.wantOutcome {
				t.Fatalf("Outcome = %v, want %v", res.Outcome, tt.wantOutcome)
			}
			if got := stackCount(inv, 10); got != 10 {
				t.Fatalf("stack left = %d, want untouched 10", got)
			}
		})
	}
}

func TestUseShotNotEnoughItemsWhenDestroyFails(t *testing.T) {
	tmpl := shotTemplate(SoulShotsHandler, modelitem.CrystalD, 0)
	caster, inv, inst := newShooter(tmpl, shotWeapon(modelitem.CrystalD), 1) // the weapon takes 2

	res := UseShot(ShotUseRequest{Caster: caster, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})

	if res.Outcome != ShotNotEnoughItems {
		t.Fatalf("Outcome = %v, want ShotNotEnoughItems", res.Outcome)
	}
	if got := weaponCharges(caster); len(got) != 0 {
		t.Fatalf("weapon charges = %v, want none: a failed destroy must not leave the weapon charged (SoulShots.java:49-62)", got)
	}
	if got := stackCount(inv, 10); got != 1 {
		t.Fatalf("stack left = %d, want untouched 1", got)
	}
}

func TestUseShotAutoEnabledPropagatesToResult(t *testing.T) {
	tmpl := shotTemplate(SoulShotsHandler, modelitem.CrystalD, 0)
	for _, tc := range []struct {
		name     string
		autoID   int32
		wantAuto bool
	}{
		{"this shot on auto", tmpl.ID, true},
		{"another shot on auto", tmpl.ID + 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster, inv, inst := newShooter(tmpl, nil, 10) // unarmed: no capacity
			caster.SetAutoSoulShot(tc.autoID, true)

			res := UseShot(ShotUseRequest{Caster: caster, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})

			if res.Outcome != ShotNoCapacity {
				t.Fatalf("Outcome = %v, want ShotNoCapacity", res.Outcome)
			}
			if res.AutoEnabled != tc.wantAuto {
				t.Fatalf("AutoEnabled = %v, want %v", res.AutoEnabled, tc.wantAuto)
			}
		})
	}
}

func TestUseShotUnrelatedHandlerNotHandled(t *testing.T) {
	tmpl := shotTemplate("SomeOtherHandler", modelitem.CrystalD, 0)
	caster, inv, inst := newShooter(tmpl, shotWeapon(modelitem.CrystalD), 10)

	res := UseShot(ShotUseRequest{Caster: caster, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})

	if res.Outcome != ShotNotHandled {
		t.Fatalf("Outcome = %v, want ShotNotHandled", res.Outcome)
	}
	if got := stackCount(inv, 10); got != 10 {
		t.Fatalf("stack left = %d, want untouched 10", got)
	}
	if got := weaponCharges(caster); len(got) != 0 {
		t.Fatalf("weapon charges = %v, want none", got)
	}
}

// ---- from use_skill_ai_cast_test.go ----
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
			defs:    definitions(scroll),
			wantIDs: []modelskill.ID{2005},
		},
		{
			name: "later non-potion skills are kept in template order",
			tmpl: &modelitem.Template{
				Kind:           modelitem.KindEtcItem,
				EtcItem:        &modelitem.EtcItemDetail{Handler: ItemSkillsHandler},
				AttachedSkills: []modelitem.SkillRef{{ID: 2005, Level: 1}, {ID: 2006, Level: 1}},
			},
			defs:    definitions(scroll, other),
			wantIDs: []modelskill.ID{2005, 2006},
		},
		{
			name: "potion carried skill is left to the instant-cast path",
			tmpl: &modelitem.Template{
				Kind:           modelitem.KindEtcItem,
				EtcItem:        &modelitem.EtcItemDetail{Handler: ItemSkillsHandler},
				AttachedSkills: []modelitem.SkillRef{{ID: 2031, Level: 1}},
			},
			defs: definitions(potion),
		},
		{
			name: "non-ItemSkills handler is not handled",
			tmpl: &modelitem.Template{
				Kind:           modelitem.KindEtcItem,
				EtcItem:        &modelitem.EtcItemDetail{Handler: "SomeOtherHandler"},
				AttachedSkills: []modelitem.SkillRef{{ID: 2005, Level: 1}},
			},
			defs: definitions(scroll),
		},
		{
			name: "no attached skills is not handled",
			tmpl: &modelitem.Template{
				Kind:    modelitem.KindEtcItem,
				EtcItem: &modelitem.EtcItemDetail{Handler: ItemSkillsHandler},
			},
			defs: definitions(),
		},
		{
			name: "nil template is not handled",
			tmpl: nil,
			defs: definitions(),
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
	req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, newUseCaster(t), false)

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
	req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, newUseCaster(t), false)

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
	caster := newUseCaster(t)
	caster.UpdateShortBuff(2037, 1, 14)
	req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, false)

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
	for _, showing := range []int32{2031, 2037} {
		caster := newUseCaster(t)
		caster.UpdateShortBuff(showing, 1, 14)
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, false)

		res := Use(req)

		if !res.HasShortBuff {
			t.Fatalf("HasShortBuff = false with %d showing, want true when the new skill id is numerically >= the currently-showing one", showing)
		}
	}
}

// ---- from use_skill_summon_test.go ----
// TestUseMirrorsHerbEffectOntoSummon drives a real HEAL_PERCENT herb: the
// caster always heals, and the mirror shows as the servitor's own HP rising
// through the same ApplyEffects surface any caster drives.
func TestUseMirrorsHerbEffectOntoSummon(t *testing.T) {
	def := modelskill.Definition{ID: 100, Level: 1, Potion: true, Target: modelskill.TargetSelf, SkillType: "HEAL_PERCENT", Power: 50}
	effects := actorcast.EffectHandlers{
		Targets: skilltarget.NewRegistry(noKnownCreatures{}),
		Skills:  handlerskill.NewDefaultRegistry(),
	}

	for _, tc := range []struct {
		name       string
		etcType    modelitem.EtcItemType
		isPet      bool
		withSummon bool
		wantMirror bool
	}{
		{"herb with an active summon mirrors onto it", modelitem.EtcItemHerb, false, true, true},
		{"herb with no active summon does not mirror", modelitem.EtcItemHerb, false, false, false},
		{"herb used by a pet does not mirror onto its own summon field", modelitem.EtcItemHerb, true, true, false},
		{"non-herb potion does not mirror even with an active summon", modelitem.EtcItemPotion, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := newUseCaster(t)
			caster.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 1, MaxMP: 100, CurrentMP: 100})
			healed := 1 + caster.MaxHPValue()/2
			servitor := newServitor(t, 2, 1, 1)
			servitor.SetHP(1)
			if caster.MaxHPValue() < 2 || servitor.MaxHPValue() < 2 {
				t.Fatalf("max HP caster/servitor = %v/%v, want room for a visible heal", caster.MaxHPValue(), servitor.MaxHPValue())
			}
			req := newUseRequest(t, ItemSkillsHandler, tc.etcType, def, caster, tc.isPet)
			req.Effects = effects
			if tc.withSummon {
				req.Summon = servitor
			}

			res := Use(req)

			if res.Outcome != Applied {
				t.Fatalf("Outcome = %v, want Applied", res.Outcome)
			}
			res.Apply()
			if got := caster.HP(); got != healed {
				t.Fatalf("caster HP = %v, want %v (1 + 50%% of its max HP)", got, healed)
			}
			want := 1.0
			if tc.wantMirror {
				want += servitor.MaxHPValue() / 2
			}
			if got := servitor.HP(); got != want {
				t.Fatalf("servitor HP = %v, want %v (1, plus 50%% of its max HP when mirrored)", got, want)
			}
		})
	}
}

type noKnownCreatures struct{}

func (noKnownCreatures) ForEachKnownCreatureInRadius(skilltarget.Actor, int, func(skilltarget.Actor)) {
}

// ---- from use_skill_test.go ----
// newUseCaster returns a live player whose clock stands still, so a reuse
// it installs stays in force for the whole test.
func newUseCaster(t *testing.T) *player.Character {
	t.Helper()
	ch := newPlayer(1, nil)
	goLive(t, ch)
	return ch
}

// useStackID is the object id of the used item's 5-unit stack.
const useStackID = 10

func newUseRequest(t *testing.T, handler string, etcType modelitem.EtcItemType, def modelskill.Definition, caster *player.Character, isPet bool) UseRequest {
	t.Helper()
	tmpl := &modelitem.Template{
		ID:          1,
		Kind:        modelitem.KindEtcItem,
		Tradable:    true,
		Stackable:   true,
		Destroyable: true,
		EtcItem: &modelitem.EtcItemDetail{
			Type: etcType, Handler: handler, SharedReuseGroup: -1,
		},
		AttachedSkills: []modelitem.SkillRef{{ID: int32(def.ID), Level: int32(def.Level)}},
	}
	inv := itemcontainer.RestorePlayerInventory(2, modelitem.NewTable([]*modelitem.Template{tmpl}), []*modelitem.Instance{carried(useStackID, tmpl.ID, 5)})

	return UseRequest{
		Caster:      caster,
		Inventory:   inv,
		Item:        inv.ItemByObjectID(useStackID),
		Definitions: definitions(def),
		Effects:     actorcast.EffectHandlers{},
		Destroyer:   destroyer(),
		IsPet:       isPet,
	}
}

func TestUse(t *testing.T) {
	potion := modelskill.Definition{ID: 100, Level: 1, Potion: true, ReuseDelay: 0}

	t.Run("potion consumes one unit", func(t *testing.T) {
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, newUseCaster(t), false)

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied", res.Outcome)
		}
		if got := stackCount(req.Inventory, useStackID); got != 4 {
			t.Fatalf("stack left = %d, want 4", got)
		}
	})

	t.Run("herb applies without consuming", func(t *testing.T) {
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, potion, newUseCaster(t), false)

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied", res.Outcome)
		}
		if got := stackCount(req.Inventory, useStackID); got != 5 {
			t.Fatalf("stack left = %d, want untouched 5 (herb must not consume)", got)
		}
	})

	t.Run("herb with a summon reports it as MirroredSummon", func(t *testing.T) {
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, potion, newUseCaster(t), false)
		servitor := newServitor(t, 2, 1, 1)
		req.Summon = servitor

		res := Use(req)

		if res.MirroredSummon != servitor {
			t.Fatalf("MirroredSummon = %v, want the servitor", res.MirroredSummon)
		}
	})

	t.Run("herb with no summon reports no MirroredSummon", func(t *testing.T) {
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, potion, newUseCaster(t), false)

		res := Use(req)

		if res.MirroredSummon != nil {
			t.Fatalf("MirroredSummon = %v, want nil", res.MirroredSummon)
		}
	})

	t.Run("potion (non-herb) with a summon does not mirror", func(t *testing.T) {
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, newUseCaster(t), false)
		req.Summon = newServitor(t, 2, 1, 1)

		res := Use(req)

		if res.MirroredSummon != nil {
			t.Fatalf("MirroredSummon = %v, want nil (non-herb never mirrors)", res.MirroredSummon)
		}
	})

	t.Run("herb used by a pet caster does not mirror", func(t *testing.T) {
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, potion, newUseCaster(t), true)
		req.Summon = newServitor(t, 2, 1, 1)

		res := Use(req)

		if res.MirroredSummon != nil {
			t.Fatalf("MirroredSummon = %v, want nil (pet caster never mirrors)", res.MirroredSummon)
		}
	})

	t.Run("herb not enough items never rejects (no consume attempted)", func(t *testing.T) {
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, potion, newUseCaster(t), false)
		// The herb is no longer carried, so a destroy would fail.
		req.Item = carried(useStackID+1, 1, 1)

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied", res.Outcome)
		}
	})

	t.Run("potion not carried is rejected", func(t *testing.T) {
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, newUseCaster(t), false)
		req.Item = carried(useStackID+1, 1, 1)

		res := Use(req)

		if res.Outcome != NotEnoughItems {
			t.Fatalf("Outcome = %v, want NotEnoughItems", res.Outcome)
		}
		if got := stackCount(req.Inventory, useStackID); got != 5 {
			t.Fatalf("carried stack left = %d, want untouched 5", got)
		}
	})

	t.Run("elixir applied for a player caster", func(t *testing.T) {
		req := newUseRequest(t, ElixirsHandler, modelitem.EtcItemElixir, potion, newUseCaster(t), false)

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied", res.Outcome)
		}
		if got := stackCount(req.Inventory, useStackID); got != 4 {
			t.Fatalf("stack left = %d, want 4", got)
		}
	})

	t.Run("elixir rejects a pet caster", func(t *testing.T) {
		req := newUseRequest(t, ElixirsHandler, modelitem.EtcItemElixir, potion, newUseCaster(t), true)

		res := Use(req)

		if res.Outcome != PetRejected {
			t.Fatalf("Outcome = %v, want PetRejected", res.Outcome)
		}
		if got := stackCount(req.Inventory, useStackID); got != 5 {
			t.Fatalf("stack left = %d, want untouched 5 (rejected before consume)", got)
		}
	})

	t.Run("plain ItemSkills item ignores IsPet", func(t *testing.T) {
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, newUseCaster(t), true)

		res := Use(req)

		if res.Outcome != Applied {
			t.Fatalf("Outcome = %v, want Applied (ItemSkillsHandler doesn't gate on IsPet)", res.Outcome)
		}
	})

	t.Run("pet rejects non-tradable herb", func(t *testing.T) {
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, potion, newUseCaster(t), true)
		tmpl, _ := req.Inventory.Templates().Get(req.Item.TemplateID)
		tmpl.Tradable = false

		res := Use(req)

		if res.Outcome != PetRejected {
			t.Fatalf("Outcome = %v, want PetRejected", res.Outcome)
		}
		if got := stackCount(req.Inventory, useStackID); got != 5 {
			t.Fatalf("stack left = %d, want untouched 5", got)
		}
	})

	t.Run("reuse rejected before consume", func(t *testing.T) {
		caster := newUseCaster(t)
		caster.DisableSkill(actorcast.ReuseKey(potion), time.Hour)
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, caster, false)

		res := Use(req)

		if res.Outcome != ReuseRejected {
			t.Fatalf("Outcome = %v, want ReuseRejected", res.Outcome)
		}
		if got := stackCount(req.Inventory, useStackID); got != 5 {
			t.Fatalf("stack left = %d, want untouched 5", got)
		}
	})

	t.Run("reports shared reuse group and the longer of skill/item reuse delay", func(t *testing.T) {
		def := modelskill.Definition{ID: 101, Level: 1, Potion: true, ReuseDelay: 1000}
		caster := newUseCaster(t)
		tmpl := &modelitem.Template{
			ID:          1,
			Kind:        modelitem.KindEtcItem,
			Stackable:   true,
			Destroyable: true,
			EtcItem: &modelitem.EtcItemDetail{
				Type: modelitem.EtcItemPotion, Handler: ItemSkillsHandler,
				ReuseDelay: 3000, SharedReuseGroup: 7,
			},
			AttachedSkills: []modelitem.SkillRef{{ID: int32(def.ID), Level: int32(def.Level)}},
		}
		inv := itemcontainer.RestorePlayerInventory(2, modelitem.NewTable([]*modelitem.Template{tmpl}), []*modelitem.Instance{carried(useStackID, tmpl.ID, 5)})
		req := UseRequest{
			Caster: caster, Inventory: inv, Item: inv.ItemByObjectID(useStackID),
			Definitions: definitions(def), Effects: actorcast.EffectHandlers{}, Destroyer: destroyer(),
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
		key := actorcast.ReuseKey(def)
		if !caster.SkillDisabled(key) || !caster.HasSkillReuse(key) {
			t.Fatalf("SkillDisabled/HasSkillReuse = %v/%v, want the item reuse installed", caster.SkillDisabled(key), caster.HasSkillReuse(key))
		}
	})

	t.Run("no shared reuse group reports -1", func(t *testing.T) {
		req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemPotion, potion, newUseCaster(t), false)

		res := Use(req)

		if res.SharedReuseGroup != -1 {
			t.Fatalf("SharedReuseGroup = %d, want -1 (template default)", res.SharedReuseGroup)
		}
	})

	t.Run("unrelated handler not handled", func(t *testing.T) {
		req := newUseRequest(t, "SomeOtherHandler", modelitem.EtcItemNone, potion, newUseCaster(t), false)

		res := Use(req)

		if res.Outcome != NotHandled {
			t.Fatalf("Outcome = %v, want NotHandled", res.Outcome)
		}
		if got := stackCount(req.Inventory, useStackID); got != 5 {
			t.Fatalf("stack left = %d, want untouched 5", got)
		}
	})
}

// newHerbPairRequest is a herb carrying first then second.
func newHerbPairRequest(t *testing.T, first, second modelskill.Definition, caster *player.Character, isPet bool) UseRequest {
	t.Helper()
	req := newUseRequest(t, ItemSkillsHandler, modelitem.EtcItemHerb, first, caster, isPet)
	tmpl, _ := req.Inventory.Templates().Get(req.Item.TemplateID)
	tmpl.AttachedSkills = []modelitem.SkillRef{{ID: int32(first.ID), Level: int32(first.Level)}, {ID: int32(second.ID), Level: int32(second.Level)}}
	req.Definitions = definitions(first, second)
	return req
}

func TestUseAll(t *testing.T) {
	first := modelskill.Definition{ID: 100, Level: 1, Potion: true, ReuseDelay: 1}
	second := modelskill.Definition{ID: 101, Level: 1, Potion: true, ReuseDelay: 1}

	t.Run("applies each attached instant skill in order", func(t *testing.T) {
		caster := newUseCaster(t)

		results := UseAll(newHerbPairRequest(t, first, second, caster, false))

		if len(results) != 2 || results[0].Skill.ID != first.ID || results[1].Skill.ID != second.ID {
			t.Fatalf("results = %#v, want both skills in attached order", results)
		}
		for _, def := range []modelskill.Definition{first, second} {
			if !caster.HasSkillReuse(actorcast.ReuseKey(def)) {
				t.Fatalf("skill %d has no reuse recorded, want each applied skill's reuse installed", def.ID)
			}
		}
	})

	t.Run("stops at the first reuse-disabled skill", func(t *testing.T) {
		caster := newUseCaster(t)
		caster.DisableSkill(actorcast.ReuseKey(first), time.Hour)

		results := UseAll(newHerbPairRequest(t, first, second, caster, false))

		if len(results) != 1 || results[0].Outcome != ReuseRejected || results[0].Skill.ID != first.ID {
			t.Fatalf("results = %#v, want first skill reuse rejection only", results)
		}
		for _, def := range []modelskill.Definition{first, second} {
			if caster.HasSkillReuse(actorcast.ReuseKey(def)) {
				t.Fatalf("skill %d has a reuse recorded, want none installed", def.ID)
			}
		}
	})
}

func TestUseAllStopsWhenSkillConditionFails(t *testing.T) {
	first := modelskill.Definition{
		ID: 100, Level: 1, Potion: true, ReuseDelay: 1000,
		Conditions: []modelskill.ConditionClause{{
			Root: modelskill.Condition{Kind: "player", Attrs: map[string]string{"flying": "false"}},
		}},
	}
	second := modelskill.Definition{ID: 101, Level: 1, Potion: true}
	for _, isPet := range []bool{false, true} {
		t.Run(map[bool]string{false: "player herb", true: "pet herb"}[isPet], func(t *testing.T) {
			caster := newUseCaster(t)
			caster.SetFlying(true)
			req := newHerbPairRequest(t, first, second, caster, isPet)

			results := UseAll(req)

			if len(results) != 1 || results[0].Outcome != ConditionRejected || results[0].Skill.ID != first.ID {
				t.Fatalf("results = %#v, want first skill condition rejection only", results)
			}
			if results[0].Condition.Root.Kind != "player" {
				t.Fatalf("failed condition = %#v, want the skill's player condition", results[0].Condition)
			}
			if got := stackCount(req.Inventory, useStackID); got != 5 {
				t.Fatalf("stack left = %d, want untouched 5", got)
			}
			if caster.HasSkillReuse(actorcast.ReuseKey(first)) {
				t.Fatal("reuse recorded after a condition failure, want none")
			}
		})
	}
}

// The item handlers act on production types here: the inventory service
// that destroys the consumed item, a *player.Character casting or charging
// its weapon, and a *summon.Actor taking a beast shot. Each test asserts
// what changed on them (the stack left, the charge on the weapon, the
// installed reuse) rather than how often a stand-in was called. See
// docs/agents/test-strategy.md.

// destroyer is the inventory service that consumes an item for real.
func destroyer() *invops.Service { return invops.NewService(nil) }

// stackCount is the count of objectID left in inv, 0 once it is gone.
func stackCount(inv *itemcontainer.Inventory, objectID int32) int {
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return 0
	}
	return inst.Snapshot().Count
}

// carried returns a carried stack of count units of templateID.
func carried(objectID, templateID int32, count int) *modelitem.Instance {
	return &modelitem.Instance{ObjectID: objectID, TemplateID: templateID, Count: count, Location: modelitem.LocationInventory}
}

// equipped returns templateID worn in the right hand.
func equipped(objectID, templateID int32) *modelitem.Instance {
	return &modelitem.Instance{ObjectID: objectID, TemplateID: templateID, Count: 1, Location: modelitem.LocationPaperdoll, LocationData: itemcontainer.RHand}
}

// definitions is the production skill table holding defs.
func definitions(defs ...modelskill.Definition) *modelskill.Table {
	return modelskill.NewTable(defs)
}

// newPlayer returns a character carrying items out of templates, with full
// 100 HP/MP.
func newPlayer(id int32, templates []*modelitem.Template, items ...*modelitem.Instance) *player.Character {
	ch := &player.Character{ID: id}
	ch.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 100, CurrentMP: 100})
	ch.AttachRuntime(&player.Template{}, itemcontainer.RestorePlayerInventory(id, modelitem.NewTable(templates), items))
	return ch
}

// goLive gives ch a creature runtime on an idle queue, which its timed
// state (the short-buff HUD countdown) is scheduled on.
func goLive(t *testing.T, ch *player.Character) {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 100, openGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(sim.NewInline(time.Unix(0, 0)).NewQueue("test"))
	ch.Attach(live, nil)
}

// newServitor returns a live servitor charging ss beast soulshots and sps
// beast spiritshots per charge.
func newServitor(t *testing.T, id int32, ss, sps int) *summon.Actor {
	t.Helper()
	s, err := summon.NewServitor(summon.ServitorConfig{ObjectID: id, Stats: summon.CombatStats{MaxHP: 100, MaxMP: 100, SSCount: ss, SPSCount: sps}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// openGeo is a move.Geo with no walls, needed only because creature.NewLive
// requires one.
type openGeo struct{}

func (openGeo) CanMove(int, int, int, int, int, int) bool { return true }

func (openGeo) Height(_, _, z int) int16 { return int16(z) }

func (openGeo) FindPath(location.Location, location.Location) ([]location.Location, bool) {
	return nil, false
}

func (openGeo) Walkable(int, int, int) bool { return true }

func (openGeo) CanFly(int, int, int, float64, int, int, int) bool { return true }

func (openGeo) ValidFlyLocation(_, _, _ int, _ float64, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

func (openGeo) ValidLocation(_, _, _, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

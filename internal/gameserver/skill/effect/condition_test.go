package effect

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// fakeConditionActor is a minimal stat.Actor+conditions.Actor/PlayerActor
// double for exercising the condition bridge without a real
// *player.Character: conditionGate.Test takes a stat.Actor and type-asserts
// it to conditions.Actor internally, matching how every production effector
// (characterStatActor/hostileStatActor/summonStatActor) implements both.
type fakeConditionActor struct {
	level       int
	moving      bool
	wearingMask int
}

func (a fakeConditionActor) STR() int                                { return 0 }
func (a fakeConditionActor) CON() int                                { return 0 }
func (a fakeConditionActor) DEX() int                                { return 0 }
func (a fakeConditionActor) INT() int                                { return 0 }
func (a fakeConditionActor) WIT() int                                { return 0 }
func (a fakeConditionActor) MEN() int                                { return 0 }
func (a fakeConditionActor) LevelMod() float64                       { return 1 }
func (a fakeConditionActor) IsSummon() bool                          { return false }
func (a fakeConditionActor) Level() int                              { return a.level }
func (a fakeConditionActor) HPRatio() float64                        { return 1 }
func (a fakeConditionActor) MPRatio() float64                        { return 1 }
func (a fakeConditionActor) X() int                                  { return 0 }
func (a fakeConditionActor) Y() int                                  { return 0 }
func (a fakeConditionActor) Z() int                                  { return 0 }
func (a fakeConditionActor) IsMoving() bool                          { return a.moving }
func (a fakeConditionActor) IsRunning() bool                         { return false }
func (a fakeConditionActor) IsRiding() bool                          { return false }
func (a fakeConditionActor) IsFlying() bool                          { return false }
func (a fakeConditionActor) IsBehind(other conditions.Actor) bool    { return false }
func (a fakeConditionActor) IsInFrontOf(other conditions.Actor) bool { return false }
func (a fakeConditionActor) ActiveSkillLevel(id int) (int, bool)     { return 0, false }
func (a fakeConditionActor) ActiveEffectLevel(id int) (int, bool)    { return 0, false }
func (a fakeConditionActor) IsSitting() bool                         { return false }
func (a fakeConditionActor) IsInOlympiadMode() bool                  { return false }
func (a fakeConditionActor) IsHero() bool                            { return false }
func (a fakeConditionActor) PkKills() int                            { return 0 }
func (a fakeConditionActor) PledgeClass() int                        { return 0 }
func (a fakeConditionActor) IsClanLeader() bool                      { return false }
func (a fakeConditionActor) HasClan() bool                           { return false }
func (a fakeConditionActor) ClanCastleID() int                       { return 0 }
func (a fakeConditionActor) ClanHasAnyCastle() bool                  { return false }
func (a fakeConditionActor) ClanHallID() int                         { return 0 }
func (a fakeConditionActor) ClanHasAnyClanHall() bool                { return false }
func (a fakeConditionActor) Race() int                               { return 0 }
func (a fakeConditionActor) Sex() int                                { return 0 }
func (a fakeConditionActor) WeightPenalty() int                      { return 0 }
func (a fakeConditionActor) InventorySize() int                      { return 0 }
func (a fakeConditionActor) InventoryLimit() int                     { return 0 }
func (a fakeConditionActor) Charges() int                            { return 0 }
func (a fakeConditionActor) IsWearingType(mask int) bool             { return a.wearingMask&mask != 0 }

func TestFuncConditionUsingItemType(t *testing.T) {
	// <add stat="rEvas" val="3"><using kind="LIGHT" /></add>, matching
	// skill 142 (Armor Mastery)'s rEvas func.
	direct := modelskill.Condition{Kind: "using", Attrs: map[string]string{"kind": "LIGHT"}}
	cond, err := funcCondition(&direct, nil)
	if err != nil {
		t.Fatalf("funcCondition: %v", err)
	}

	const lightMask = 1 << 15 // item.ArmorLight.Mask(): weaponTypeCount(14) + ArmorLight(1)
	wearingLight := fakeConditionActor{wearingMask: lightMask}
	wearingHeavy := fakeConditionActor{wearingMask: 1 << 16}

	if !cond.Test(wearingLight) {
		t.Error("using LIGHT should pass while wearing light armor")
	}
	if cond.Test(wearingHeavy) {
		t.Error("using LIGHT should fail while wearing heavy armor")
	}
}

func TestFuncConditionPlayerAndComposition(t *testing.T) {
	// <basemul stat="cAtkPos" val="0.3"><and><player moving="true"/></and></add>
	direct := modelskill.Condition{
		Kind: "and",
		Children: []modelskill.Condition{
			{Kind: "player", Attrs: map[string]string{"moving": "true"}},
		},
	}
	cond, err := funcCondition(&direct, nil)
	if err != nil {
		t.Fatalf("funcCondition: %v", err)
	}

	moving := fakeConditionActor{moving: true}
	still := fakeConditionActor{moving: false}

	if !cond.Test(moving) {
		t.Error("and{player moving=true} should pass while moving")
	}
	if cond.Test(still) {
		t.Error("and{player moving=true} should fail while not moving")
	}
}

func TestFuncConditionDirectAndAttachAreANDed(t *testing.T) {
	direct := modelskill.Condition{Kind: "player", Attrs: map[string]string{"moving": "true"}}
	attach := &modelskill.ConditionClause{
		Root: modelskill.Condition{Kind: "using", Attrs: map[string]string{"kind": "LIGHT"}},
	}
	cond, err := funcCondition(&direct, attach)
	if err != nil {
		t.Fatalf("funcCondition: %v", err)
	}

	const lightMask = 1 << 15
	both := fakeConditionActor{moving: true, wearingMask: lightMask}
	onlyMoving := fakeConditionActor{moving: true}
	onlyWearing := fakeConditionActor{wearingMask: lightMask}

	if !cond.Test(both) {
		t.Error("both direct and attach conditions satisfied should pass")
	}
	if cond.Test(onlyMoving) {
		t.Error("attach condition (wearing light) unmet should fail")
	}
	if cond.Test(onlyWearing) {
		t.Error("direct condition (moving) unmet should fail")
	}
}

func TestFuncConditionUnsupportedTagErrors(t *testing.T) {
	direct := modelskill.Condition{Kind: "targetplayable"}
	if _, err := funcCondition(&direct, nil); err == nil {
		t.Error("unsupported condition tag should error, not silently pass")
	}
}

// notConditionActor is a stat.Actor that doesn't also satisfy
// conditions.Actor, exercising conditionGate's fail-closed path.
type notConditionActor struct{}

func (notConditionActor) STR() int          { return 0 }
func (notConditionActor) CON() int          { return 0 }
func (notConditionActor) DEX() int          { return 0 }
func (notConditionActor) INT() int          { return 0 }
func (notConditionActor) WIT() int          { return 0 }
func (notConditionActor) MEN() int          { return 0 }
func (notConditionActor) Level() int        { return 0 }
func (notConditionActor) LevelMod() float64 { return 1 }
func (notConditionActor) IsSummon() bool    { return false }

var _ stat.Actor = notConditionActor{}

func TestConditionGateFailsClosedWithoutActor(t *testing.T) {
	direct := modelskill.Condition{Kind: "player", Attrs: map[string]string{"moving": "true"}}
	cond, err := funcCondition(&direct, nil)
	if err != nil {
		t.Fatalf("funcCondition: %v", err)
	}

	if cond.Test(notConditionActor{}) {
		t.Error("conditionGate should fail closed when effector doesn't satisfy conditions.Actor")
	}
}

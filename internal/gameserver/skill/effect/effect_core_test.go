package effect

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// ---- from condition_test.go ----
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

// ---- from core_test.go ----
type funcOwner struct {
	funcs []Mod
}

func (o *funcOwner) AddStatFuncs(funcs []Mod) {
	o.funcs = append(o.funcs, funcs...)
}

func (o *funcOwner) RemoveStatsByOwner(ModOwner) {}

func (o *funcOwner) MaxBuffCount() int { return 20 }

// The effect names, type strings, flags, stat-func mapping, and DoT branch
// expectations below were generated from the reference effect classes with
// actor/network dependencies replaced by scalar inputs or metadata dumps.

func TestNewBuildsBuffWithRuntimeStatFuncs(t *testing.T) {
	skill := Skill{ID: 1204}
	tmpl := modelskill.EffectTemplate{
		Name:       "Buff",
		StackType:  "speed",
		StackOrder: 1,
		Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "runSpd", Value: 33},
			{Op: modelskill.FuncMul, Stat: "pAtk", Value: 1.2},
		},
	}

	e, err := New(skill, tmpl)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if e.Type != TypeBuff {
		t.Fatalf("Type = %s, want %s", e.Type, TypeBuff)
	}
	if e.Skill.Debuff {
		t.Fatal("buff was marked as debuff")
	}
	if len(e.Funcs) != 2 {
		t.Fatalf("Funcs length = %d, want 2", len(e.Funcs))
	}
	if e.Funcs[0].Owner != ModOwnerEffect(e) {
		t.Fatal("compiled func owner is not the runtime effect")
	}
	if e.Funcs[0].Stat != stat.RunSpeed {
		t.Fatalf("first func stat = %s, want runSpd", e.Funcs[0].Stat)
	}
	if got := apply(e.Funcs[0], nil, 100, 100); got != 133 {
		t.Fatalf("first func apply() = %v, want 133", got)
	}

	owner := &funcOwner{}
	NewList(owner).Add(e)
	if !reflect.DeepEqual(owner.funcs, e.Funcs) {
		t.Fatalf("owner funcs = %#v, want effect funcs", owner.funcs)
	}
}

func TestNewBuildsCoreEffectMetadata(t *testing.T) {
	tests := []struct {
		name        string
		wantType    Type
		wantFlag    Flag
		debuff      bool
		wantRejects bool
	}{
		{"Debuff", TypeDebuff, FlagNone, false, false},
		{"Stun", TypeStun, FlagStunned, false, true},
		{"Root", TypeRoot, FlagRooted, false, true},
		{"Sleep", TypeSleep, FlagSleep, false, true},
		{"Fear", TypeFear, FlagFear, false, true},
		{"DamOverTime", TypeDamOverTime, FlagNone, false, false},
		{"ManaDamOverTime", TypeManaDamOverTime, FlagNone, false, false},
		{"AbortCast", TypeAbortCast, FlagNone, false, false},
		{"ImmobileUntilAttacked", TypeImmobileUntilAttacked, FlagMeditating, false, false},
		{"ImobileBuff", TypeImmobilizeEffector, FlagNone, false, false},
		{"Invincible", TypeInvincible, FlagNone, false, false},
		{"ManaHealOverTime", TypeManaHealOverTime, FlagNone, false, false},
		{"Mute", TypeMute, flagMuted, false, false},
		{"NoblesseBless", TypeNoblesseBless, flagNoblesseBlessing, false, false},
		{"Paralyze", TypeParalyze, FlagParalyzed, false, false},
		{"Petrification", TypePetrification, FlagParalyzed, false, false},
		{"PhysicalMute", TypePhysicalMute, flagPhysicalMuted, false, false},
		{"RemoveTarget", TypeRemoveTarget, FlagNone, false, false},
		{"SilenceMagicPhysical", TypeSilenceAll, flagMuted | flagPhysicalMuted, false, false},
		{"SilentMove", TypeSilentMove, FlagSilentMove, false, false},
		{"StunSelf", TypeStunSelf, FlagStunned, false, false},
		{"Heal", TypeHeal, FlagNone, false, false},
		{"HealOverTime", TypeHealOverTime, FlagNone, false, false},
		{"ManaHeal", TypeManaHeal, FlagNone, false, false},
		{"TargetMe", TypeTargetMe, FlagNone, false, false},
		{"Bluff", TypeBluff, FlagNone, false, false},
		{"CharmOfCourage", TypeCharmOfCourage, flagCharmOfCourage, false, false},
		{"CharmOfLuck", TypeCharmOfLuck, FlagCharmOfLuck, false, false},
		{"PhoenixBless", TypePhoenixBless, FlagPhoenixBlessing, false, false},
		{"BlockBuff", TypeBlockBuff, FlagNone, false, false},
		{"BlockDebuff", TypeBlockDebuff, FlagNone, false, false},
		{"ProtectionBlessing", TypeProtectionBless, flagProtectionBlessing, false, false},
		{"PolearmTargetSingle", TypePolearmTargetSingle, FlagNone, false, false},
		{"BigHead", TypeBigHead, flagBigHead, false, false},
		{"Spoil", TypeSpoil, FlagNone, false, false},
		{"CancelDebuff", TypeCancelDebuff, FlagNone, false, false},
		{"ImobilePetBuff", TypeImmobilizePetBuff, FlagNone, false, false},
		{"Distrust", TypeDistrust, FlagNone, false, false},
		{"Confusion", TypeConfusion, FlagConfused, false, false},
		{"Betray", TypeBetray, FlagBetrayed, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: tt.name})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			if e.Type != tt.wantType {
				t.Fatalf("Type = %s, want %s", e.Type, tt.wantType)
			}
			if e.Flag != tt.wantFlag {
				t.Fatalf("Flag = %v, want %v", e.Flag, tt.wantFlag)
			}
			if e.Skill.Debuff != tt.debuff {
				t.Fatalf("Debuff = %v, want %v", e.Skill.Debuff, tt.debuff)
			}
			if e.RejectsIfAffected != tt.wantRejects {
				t.Fatalf("RejectsIfAffected = %v, want %v", e.RejectsIfAffected, tt.wantRejects)
			}
			if e.ActionTime() {
				t.Fatal("non-periodic action hook continued")
			}
		})
	}
}

func TestNewPreservesDatapackDebuffFlag(t *testing.T) {
	e, err := New(Skill{ID: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Stun"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if !e.Skill.Debuff {
		t.Fatal("Debuff = false, want datapack value preserved")
	}
}

func TestParalyzeAndPetrificationExitDoNotThinkPlayers(t *testing.T) {
	for _, name := range []string{"Paralyze", "Petrification"} {
		t.Run(name, func(t *testing.T) {
			target := &liveEffectTarget{isPlayer: true}
			e, err := New(Skill{}, modelskill.EffectTemplate{Name: name})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			e.Effected = target
			e.OnExit(e)
			for _, event := range target.events {
				if event == "think" {
					t.Fatalf("exit events = %#v, want no THINK for a player", target.events)
				}
			}
		})
	}
}

// TestNewDerivesHerbFromSkillName mirrors AbstractEffect._isHerbEffect =
// _skill.getName().contains("Herb"): Herb is a property of the skill's name,
// not of how the effect was applied, so a skill named "Herb of Life" is a
// herb effect on any cast path, and an unrelated buff never is.
func TestNewDerivesHerbFromSkillName(t *testing.T) {
	e, err := New(Skill{ID: 1, Name: "Herb of Life"}, modelskill.EffectTemplate{Name: "Buff"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if !e.Herb {
		t.Fatal("Herb = false, want true for a skill named \"Herb of Life\"")
	}

	e, err = New(Skill{ID: 2, Name: "Wind Strike"}, modelskill.EffectTemplate{Name: "Buff"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if e.Herb {
		t.Fatal("Herb = true, want false for an unrelated skill name")
	}
}

func TestClassTagPrefersAttributeThenKind(t *testing.T) {
	// A marker effect loaded from a datapack <effect name="BlockBuff"> carries
	// no effectType attribute, so its classification is the runtime kind.
	withoutAttr, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "BlockBuff"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if got := withoutAttr.ClassTag(); got != "BLOCK_BUFF" {
		t.Fatalf("ClassTag() = %q, want %q", got, "BLOCK_BUFF")
	}

	// An explicit datapack effectType attribute overrides the kind, the same
	// reclassification used to tag a plain Buff as BLOCK_DEBUFF in tests.
	withAttr, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "Buff", EffectType: "BLOCK_DEBUFF"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if got := withAttr.ClassTag(); got != "BLOCK_DEBUFF" {
		t.Fatalf("ClassTag() = %q, want %q", got, "BLOCK_DEBUFF")
	}
}

func TestPolearmTargetSingleEffectCarriesNoHooks(t *testing.T) {
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "PolearmTargetSingle"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if e.OnStart != nil || e.OnExit != nil {
		t.Fatal("PolearmTargetSingle must carry no start/exit hooks, only a classification marker")
	}
	if e.Flag != FlagNone {
		t.Fatalf("Flag = %v, want FlagNone", e.Flag)
	}
}

func TestBigHeadEffectCarriesVisibleAbnormalHooks(t *testing.T) {
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "BigHead"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if e.OnStart == nil || e.OnExit == nil {
		t.Fatal("BigHead must start and stop its visible abnormal effect")
	}
	if e.Flag == FlagNone {
		t.Fatal("BigHead must carry a distinct, non-zero flag")
	}
}

// TestBigHeadEffectTogglesMaskAndBroadcastsDistinctFromIconRefresh proves
// BigHead's start/exit hooks flip the 0x002000 mask and drive both the icon
// refresh (UpdateAbnormalEffect, Java's EffectList.updateEffectIcons()) and
// the client-visible mask broadcast (BroadcastAbnormalEffect, Java's
// Creature.startAbnormalEffect() -> updateAbnormalEffect()) — the gap this
// issue closes for player targets.
func TestBigHeadEffectTogglesMaskAndBroadcastsDistinctFromIconRefresh(t *testing.T) {
	target := &growEffectTarget{}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "BigHead"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("OnStart() = false, want true")
	}
	e.OnExit(e)

	want := []string{"start:0x2000", "abnormal", "broadcast", "stop:0x2000", "abnormal", "broadcast"}
	if !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events = %#v, want %#v", target.events, want)
	}
}

func TestSignetGroundKindsAcceptButDeclineToStartOutsideALiveCast(t *testing.T) {
	for _, name := range []string{"Signet", "SignetNoise", "SignetAntiSummon", "SignetMDam"} {
		e, err := New(Skill{}, modelskill.EffectTemplate{Name: name})
		if err != nil {
			t.Fatalf("New(%q) error: %v, want a shipped signet template to be accepted", name, err)
		}
		if e.Type != TypeSignetGround {
			t.Fatalf("New(%q).Type = %s, want %s", name, e.Type, TypeSignetGround)
		}
		if e.OnStart == nil {
			t.Fatalf("New(%q) carries no OnStart hook", name)
		}
		if e.OnStart(e) {
			t.Fatalf("New(%q).OnStart() = true, want false: no actor exists to drive it outside handler/skill/signet.go's live-cast dispatch", name)
		}
	}
}

func TestClanGateEffectStartsAndStopsMagicCircle(t *testing.T) {
	target := &growEffectTarget{}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "ClanGate"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("ClanGate OnStart() = false, want true")
	}
	want := []string{fmt.Sprintf("start:%#x", magicCircleAbnormalMask), "abnormal", "broadcast"}
	if !reflect.DeepEqual(target.events, want) {
		t.Fatalf("start events = %#v, want %#v", target.events, want)
	}

	e.OnExit(e)
	want = append(want, fmt.Sprintf("stop:%#x", magicCircleAbnormalMask), "abnormal", "broadcast")
	if !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events after exit = %#v, want %#v", target.events, want)
	}
}

func TestFusionEffectActionNeverEndsOnItsOwnTick(t *testing.T) {
	e, err := New(Skill{Level: 3}, modelskill.EffectTemplate{Name: "Fusion", Time: 15})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true: a fusion effect never ends via its own periodic tick")
	}
}

func TestFusionEffectIncreaseEffectGrowsLevelAndReapplies(t *testing.T) {
	target := &liveEffectTarget{list: NewList(nil)}
	e, err := New(Skill{Level: 3}, modelskill.EffectTemplate{Name: "Fusion", Time: 15})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)

	var reappliedAt int
	e.IncreaseEffect(target.list, 5, func(level int) { reappliedAt = level })

	if e.Level != 4 {
		t.Fatalf("Level after IncreaseEffect = %d, want 4", e.Level)
	}
	if reappliedAt != 4 {
		t.Fatalf("reapply level = %d, want 4", reappliedAt)
	}
	if hasEffectInList(target.list, e) {
		t.Error("the prior instance must be removed before its replacement is applied")
	}
}

func TestFusionEffectIncreaseEffectAtMaxLevelIsANoop(t *testing.T) {
	target := &liveEffectTarget{list: NewList(nil)}
	e, err := New(Skill{Level: 5}, modelskill.EffectTemplate{Name: "Fusion", Time: 15})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)

	reapplied := false
	e.IncreaseEffect(target.list, 5, func(int) { reapplied = true })

	if e.Level != 5 {
		t.Fatalf("Level after IncreaseEffect at max = %d, want unchanged 5", e.Level)
	}
	if reapplied {
		t.Error("IncreaseEffect at max level must not reapply")
	}
	if !hasEffectInList(target.list, e) {
		t.Error("IncreaseEffect at max level must leave the instance in place")
	}
}

func TestFusionEffectDecreaseForceShrinksLevelAndReapplies(t *testing.T) {
	target := &liveEffectTarget{list: NewList(nil)}
	e, err := New(Skill{Level: 3}, modelskill.EffectTemplate{Name: "Fusion", Time: 15})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)

	var reappliedAt int
	e.DecreaseForce(target.list, func(level int) { reappliedAt = level })

	if e.Level != 2 {
		t.Fatalf("Level after DecreaseForce = %d, want 2", e.Level)
	}
	if reappliedAt != 2 {
		t.Fatalf("reapply level = %d, want 2", reappliedAt)
	}
	if hasEffectInList(target.list, e) {
		t.Error("the prior instance must be removed before its replacement is applied")
	}
}

func TestFusionEffectDecreaseForceBelowOneRemovesWithoutReapply(t *testing.T) {
	target := &liveEffectTarget{list: NewList(nil)}
	e, err := New(Skill{Level: 1}, modelskill.EffectTemplate{Name: "Fusion", Time: 15})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)

	reapplied := false
	e.DecreaseForce(target.list, func(int) { reapplied = true })

	if e.Level != 0 {
		t.Fatalf("Level after DecreaseForce below 1 = %d, want 0", e.Level)
	}
	if reapplied {
		t.Error("DecreaseForce dropping below level 1 must not reapply")
	}
	if hasEffectInList(target.list, e) {
		t.Error("DecreaseForce dropping below level 1 must remove the instance")
	}
}

func TestSeedEffectNeverEndsViaItsOwnTick(t *testing.T) {
	e, err := New(Skill{Level: 1}, modelskill.EffectTemplate{Name: "Seed", Time: 5})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if e.ActionTime() {
		t.Fatal("ActionTime() = true, want false: a seed effect has no periodic tick behavior")
	}
}

func TestSeedEffectStartsAtSkillLevel(t *testing.T) {
	e, err := New(Skill{Level: 1}, modelskill.EffectTemplate{Name: "Seed", Time: 5})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if e.Level != 1 {
		t.Fatalf("initial Level = %d, want 1 (matching EffectSeed's initial power)", e.Level)
	}
}

// TestSeedEffectPowerIgnoresSkillLevel guards EffectSeed.java:10's
// unconditional `_power = 1`: the initial charge must stay 1 even for a
// higher-level SEED skill, not track skill.getLevel() the way every other
// effect kind's Level does.
func TestSeedEffectPowerIgnoresSkillLevel(t *testing.T) {
	e, err := New(Skill{Level: 3}, modelskill.EffectTemplate{Name: "Seed", Time: 5})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if e.Level != 1 {
		t.Fatalf("initial Level = %d, want 1 regardless of skill level 3 (EffectSeed._power = 1 is hardcoded)", e.Level)
	}
}

func TestSeedEffectIncreasePowerGrowsLevelInPlace(t *testing.T) {
	target := &liveEffectTarget{list: NewList(nil)}
	e, err := New(Skill{Level: 1}, modelskill.EffectTemplate{Name: "Seed", Time: 5})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)

	e.IncreasePower()

	if e.Level != 2 {
		t.Fatalf("Level after IncreasePower = %d, want 2", e.Level)
	}
	if !hasEffectInList(target.list, e) {
		t.Error("IncreasePower must grow the same instance in place, not replace it")
	}
}

// TestSeedRecastDoesNotExtendDeadline guards against reintroducing a
// reschedule-on-recast call. AbstractEffect.rescheduleEffect() ->
// startEffectTask()'s initialDelay = max((_period - getTime())*1000, 5) is
// derived from the effect's construction-time _periodStartTime, which
// rescheduleEffect() never mutates (AbstractEffect.java:264-270, 186-206,
// 138-141) — so a recast reproduces the same original deadline instead of
// granting a fresh full period. In this port that means growing a seed's
// power in place must leave its already-set nextAction/remaining alone.
func TestSeedRecastDoesNotExtendDeadline(t *testing.T) {
	target := &liveEffectTarget{list: NewList(nil)}
	e, err := New(Skill{Level: 1}, modelskill.EffectTemplate{Name: "Seed", Time: 5})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)

	t0 := time.Now()
	e.startSchedule(t0)
	wantDeadline := e.nextAction
	wantRemaining := e.remaining

	e.IncreasePower()

	if e.nextAction != wantDeadline {
		t.Fatalf("nextAction after recast = %v, want unchanged %v (reference pins the deadline)", e.nextAction, wantDeadline)
	}
	if e.remaining != wantRemaining {
		t.Fatalf("remaining after recast = %d, want unchanged %d", e.remaining, wantRemaining)
	}
}

// ---- from helpers_test.go ----
// namedActor is a minimal participant with a fixed String() so tests that
// only need a filler Effector/Effected (not a specific capability) get a
// stable %v representation instead of a struct address.
type namedActor string

func (namedActor) ObjectID() int32  { return 0 }
func (namedActor) Dead() bool       { return false }
func (n namedActor) String() string { return string(n) }

type liveEffectTarget struct {
	events            []string
	hp                float64
	mp                float64
	dead              bool
	afraid            bool
	fearImmune        bool
	playable          bool
	raidRelated       bool
	castingNow        bool
	castMagic         bool
	canBeHealed       bool
	healProficiency   float64
	healEffectiveness float64
	rechargeRate      func(float64) float64
	target            worldobject.Object
	heading           int
	bluffExempt       bool
	isPlayer          bool
	list              *List
	vuln              float64
	standing          bool
	hpFull            bool
	relaxNotice       int
	recentFakeDeath   bool
	objectID          int32
	ownerID           int32
	x, y, z           int
	validLocationFn   func(ox, oy, oz, tx, ty, tz int) location.Location
	flightDest        location.Location
	flightType        modelskill.Flight
	mpBroadcasts      int
}

func (t *liveEffectTarget) BroadcastMPStatus() { t.mpBroadcasts++ }

func (t *liveEffectTarget) EffectList() *List { return t.list }

func (t *liveEffectTarget) CancelVulnerability(classification string) float64 { return t.vuln }

func (t *liveEffectTarget) Dead() bool { return t.dead }

func (t *liveEffectTarget) HP() float64 { return t.hp }

func (t *liveEffectTarget) MPValue() float64 { return t.mp }

func (t *liveEffectTarget) ReduceHPByDOT(damage float64, effector Participant, isDOT bool) {
	t.hp -= damage
	t.events = append(t.events, fmt.Sprintf("dot:%g:%v", damage, effector))
}

// ReduceMP mirrors the production actors' clamp-at-zero semantics (see
// Character.ReduceMP/Hostile.ReduceMP): a target already at 0 MP applies
// and returns 0 rather than going negative, so tests can exercise the
// "nothing to apply, don't broadcast" guard alongside the real reducers.
func (t *liveEffectTarget) ReduceMP(damage float64) float64 {
	if damage <= 0 || t.mp <= 0 {
		return 0
	}
	if damage > t.mp {
		damage = t.mp
	}
	t.mp -= damage
	t.events = append(t.events, fmt.Sprintf("mpdot:%g", damage))
	return damage
}

func (t *liveEffectTarget) NotifyEffectRemovedDueLackHP(*Effect) {
	t.events = append(t.events, "lack-hp")
}

func (t *liveEffectTarget) NotifyEffectRemovedDueLackMP(*Effect) {
	t.events = append(t.events, "lack-mp")
}

func (t *liveEffectTarget) AbortAll(force bool) {
	t.events = append(t.events, fmt.Sprintf("abort:%v", force))
}

func (t *liveEffectTarget) TryToIdle() {
	t.events = append(t.events, "idle")
}

func (t *liveEffectTarget) StopMove() {
	t.events = append(t.events, "stop-move")
}

func (t *liveEffectTarget) UpdateAbnormalEffect() {
	t.events = append(t.events, "abnormal")
}

func (t *liveEffectTarget) BroadcastAbnormalEffect() {
	t.events = append(t.events, "broadcast")
}

func (t *liveEffectTarget) Think() error {
	t.events = append(t.events, "think")
	return nil
}

func (t *liveEffectTarget) Afraid() bool { return t.afraid }

func (t *liveEffectTarget) FearImmune() bool { return t.fearImmune }

func (t *liveEffectTarget) Playable() bool { return t.playable }

func (t *liveEffectTarget) FleeFrom(effector Participant, distance int) {
	t.events = append(t.events, fmt.Sprintf("flee:%v:%d", effector, distance))
}

func (t *liveEffectTarget) StopEffects(typ Type) {
	t.events = append(t.events, "stop-effects:"+string(typ))
}

func (t *liveEffectTarget) RaidRelated() bool { return t.raidRelated }

func (t *liveEffectTarget) CastingNow() bool { return t.castingNow }

func (t *liveEffectTarget) CurrentSkillIsMagic() bool { return t.castMagic }

func (t *liveEffectTarget) InterruptCast() {
	t.events = append(t.events, "interrupt-cast")
}

func (t *liveEffectTarget) StopCast() {
	t.events = append(t.events, "stop-cast")
}

func (t *liveEffectTarget) ClearTarget() {
	t.events = append(t.events, "clear-target")
}

func (t *liveEffectTarget) StopAttack() {
	t.events = append(t.events, "stop-attack")
}

func (t *liveEffectTarget) SetInvul(v bool) {
	t.events = append(t.events, fmt.Sprintf("invul:%v", v))
}

func (t *liveEffectTarget) SetImmobilized(v bool) bool {
	t.events = append(t.events, fmt.Sprintf("immobilized:%v", v))
	return true
}

func (t *liveEffectTarget) CanBeHealed() bool { return t.canBeHealed }

func (t *liveEffectTarget) AddMP(amount float64) float64 {
	t.mp += amount
	t.events = append(t.events, fmt.Sprintf("add-mp:%g", amount))
	return amount
}

func (t *liveEffectTarget) AddHP(amount float64) float64 {
	t.hp += amount
	t.events = append(t.events, fmt.Sprintf("add-hp:%g", amount))
	return amount
}

func (t *liveEffectTarget) HealProficiency() float64 { return t.healProficiency }

func (t *liveEffectTarget) HealEffectiveness() float64 { return t.healEffectiveness }

func (t *liveEffectTarget) RechargeMP(base float64) float64 {
	if t.rechargeRate == nil {
		return base
	}
	return t.rechargeRate(base)
}

func (t *liveEffectTarget) CurrentTarget() worldobject.Object { return t.target }

func (t *liveEffectTarget) SetTarget(target worldobject.Object) {
	t.target = target
	t.events = append(t.events, fmt.Sprintf("set-target:%v", target))
}

func (t *liveEffectTarget) TryToAttack(target worldobject.Object) {
	t.events = append(t.events, fmt.Sprintf("try-attack:%v", target))
}

func (t *liveEffectTarget) Heading() int { return t.heading }

func (t *liveEffectTarget) SetHeading(h int) {
	t.heading = h
	t.events = append(t.events, fmt.Sprintf("heading:%d", h))
}

func (t *liveEffectTarget) BluffExempt() bool { return t.bluffExempt }

func (t *liveEffectTarget) IsPlayer() bool { return t.isPlayer }

func (t *liveEffectTarget) StopCharmOfLuck(*Effect) {
	t.events = append(t.events, "stop-charm-of-luck")
}

func (t *liveEffectTarget) StopPhoenixBlessing(*Effect) {
	t.events = append(t.events, "stop-phoenix-bless")
}

func (t *liveEffectTarget) StopSkillEffectsByID(id modelskill.ID) {
	t.events = append(t.events, fmt.Sprintf("stop-skill:%d", id))
}

func (t *liveEffectTarget) Standing() bool { return t.standing }

func (t *liveEffectTarget) SetStanding(v bool) bool {
	changed := t.standing != v
	t.standing = v
	t.events = append(t.events, fmt.Sprintf("standing:%v", v))
	return changed
}

func (t *liveEffectTarget) HPFull() bool { return t.hpFull }

func (t *liveEffectTarget) NotifyRelaxDeactivatedHPFull(*Effect) {
	t.relaxNotice++
}

func (t *liveEffectTarget) MarkRecentFakeDeath() {
	t.recentFakeDeath = true
	t.events = append(t.events, "recent-fake-death")
}

func (t *liveEffectTarget) ObjectID() int32 { return t.objectID }

func (t *liveEffectTarget) OwnerID() int32 { return t.ownerID }

func (t *liveEffectTarget) X() int { return t.x }
func (t *liveEffectTarget) Y() int { return t.y }
func (t *liveEffectTarget) Z() int { return t.z }

func (t *liveEffectTarget) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	if t.validLocationFn != nil {
		return t.validLocationFn(ox, oy, oz, tx, ty, tz)
	}
	return location.Location{X: tx, Y: ty, Z: tz}
}

func (t *liveEffectTarget) FlyTo(dest location.Location, flight modelskill.Flight) {
	t.flightDest = dest
	t.flightType = flight
	t.events = append(t.events, "fly")
}

func (t *liveEffectTarget) SetXYZ(x, y, z int) {
	t.x, t.y, t.z = x, y, z
}

func (t *liveEffectTarget) BroadcastPosition() {
	t.events = append(t.events, "broadcast")
}

// noBonusHealTarget implements only the minimum heal capability, to
// exercise the healStart/manaHealStart fallback defaults when the optional

func hasEffectInList(list *List, e *Effect) bool {
	for _, cur := range list.All() {
		if cur == e {
			return true
		}
	}
	return false
}

type growEffectTarget struct {
	events []string
	radius float64
}

func (t *growEffectTarget) ObjectID() int32 { return 0 }

func (t *growEffectTarget) Dead() bool { return false }

func (t *growEffectTarget) CollisionRadius() float64 { return t.radius }

func (t *growEffectTarget) SetCollisionRadius(radius float64) {
	t.radius = radius
	t.events = append(t.events, fmt.Sprintf("set:%g", radius))
}

func (t *growEffectTarget) ResetCollisionRadius() {
	t.events = append(t.events, "reset")
}

func (t *growEffectTarget) StartAbnormalEffect(mask int) {
	t.events = append(t.events, fmt.Sprintf("start:%#x", mask))
}

func (t *growEffectTarget) StopAbnormalEffect(mask int) {
	t.events = append(t.events, fmt.Sprintf("stop:%#x", mask))
}

func (t *growEffectTarget) UpdateAbnormalEffect() {
	t.events = append(t.events, "abnormal")
}

func (t *growEffectTarget) BroadcastAbnormalEffect() {
	t.events = append(t.events, "broadcast")
}

// ---- from hooks_chance_trigger_test.go ----
func TestNewChanceSkillTriggerRejectsUnknownTriggerType(t *testing.T) {
	if _, err := New(Skill{}, modelskill.EffectTemplate{Name: "ChanceSkillTrigger", ChanceType: "BOGUS", ActivationChance: 50}); err == nil {
		t.Fatal("New() error = nil, want an error for an unknown chanceType")
	}
}

func TestNewChanceSkillTriggerAcceptsAnAbsentChanceType(t *testing.T) {
	if _, err := New(Skill{}, modelskill.EffectTemplate{Name: "ChanceSkillTrigger", TriggeredID: 5144}); err != nil {
		t.Fatalf("New() error = %v, want nil for an absent chanceType", err)
	}
}

type chanceTriggerFakeActor struct {
	tracked []*Effect
}

func (a *chanceTriggerFakeActor) ObjectID() int32 { return 0 }

func (a *chanceTriggerFakeActor) Dead() bool { return false }

func (a *chanceTriggerFakeActor) AddChanceTrigger(e *Effect) {
	a.tracked = append(a.tracked, e)
}

func (a *chanceTriggerFakeActor) RemoveChanceTrigger(e *Effect) {
	for i, cur := range a.tracked {
		if cur == e {
			a.tracked = append(a.tracked[:i], a.tracked[i+1:]...)
			return
		}
	}
}

func TestChanceSkillTriggerInstallsAndRemovesOnTarget(t *testing.T) {
	target := &chanceTriggerFakeActor{}
	e, err := New(Skill{}, modelskill.EffectTemplate{
		Name: "ChanceSkillTrigger", Time: 60, TriggeredID: 5144,
		ChanceType: "ON_ATTACKED", ActivationChance: 80,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("OnStart() = false, want true")
	}
	if len(target.tracked) != 1 || target.tracked[0] != e {
		t.Fatalf("tracked after OnStart = %+v, want [e]", target.tracked)
	}

	e.OnExit(e)
	if len(target.tracked) != 0 {
		t.Fatalf("tracked after OnExit = %+v, want empty", target.tracked)
	}
}

func TestChanceSkillTriggerOnATargetWithNoTrackingIsANoop(t *testing.T) {
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "ChanceSkillTrigger", ChanceType: "ON_HIT", ActivationChance: 50})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if !e.OnStart(e) {
		t.Fatal("OnStart() = false, want true even without a tracking target")
	}
	e.OnExit(e)
}

// ---- from item_stats_test.go ----
func TestItemModifierFuncsBuildsAddSetAndEnchantFuncs(t *testing.T) {
	tmpl := &item.Template{
		ID:      100,
		Crystal: item.CrystalS,
		Weapon:  &item.WeaponDetail{Type: item.WeaponSword},
		Modifiers: []item.StatModifier{
			{Op: item.FuncAdd, Stat: "pAtk", Value: 10},
			{Op: item.FuncSet, Stat: "pAtkSpd", Value: 300},
			{Op: item.FuncEnchant, Stat: "pAtk", Value: 0},
		},
	}
	inst := &item.Instance{ObjectID: 1, TemplateID: 100, EnchantLevel: 4}
	owner := ItemOwner{Inst: inst, Tmpl: tmpl}

	fns, err := ItemModifierFuncs(owner)
	if err != nil {
		t.Fatalf("ItemModifierFuncs() error: %v", err)
	}
	if len(fns) != 3 {
		t.Fatalf("len(fns) = %d, want 3", len(fns))
	}
	if fns[2].Op != OpEnchant {
		t.Fatalf("fns[2].Op = %v, want OpEnchant", fns[2].Op)
	}
	for _, fn := range fns {
		if fn.Owner != ModOwnerItem(owner) {
			t.Fatalf("Owner = %v, want %v", fn.Owner, ModOwnerItem(owner))
		}
	}
}

func TestItemModifierFuncsRejectsConditionalModifier(t *testing.T) {
	tmpl := &item.Template{
		ID: 101,
		Modifiers: []item.StatModifier{
			{Op: item.FuncAdd, Stat: "pAtk", Value: 10, Condition: &item.Condition{}},
		},
	}
	owner := ItemOwner{Inst: &item.Instance{ObjectID: 1, TemplateID: 101}, Tmpl: tmpl}

	if _, err := ItemModifierFuncs(owner); err == nil {
		t.Fatal("ItemModifierFuncs() error = nil, want error for a conditional modifier")
	}
}

func TestItemOwnerEnchantLevelReadsLiveInstanceState(t *testing.T) {
	tmpl := &item.Template{ID: 102, Crystal: item.CrystalD}
	inst := &item.Instance{ObjectID: 1, TemplateID: 102, EnchantLevel: 0}
	owner := ItemOwner{Inst: inst, Tmpl: tmpl}

	if got := owner.EnchantLevel(); got != 0 {
		t.Fatalf("EnchantLevel() = %d, want 0", got)
	}
	inst.SetEnchantLevel(5)
	if got := owner.EnchantLevel(); got != 5 {
		t.Fatalf("EnchantLevel() after SetEnchantLevel(5) = %d, want 5 (must read live state, not a captured value)", got)
	}
}

func TestItemPassiveFuncsOnlyAppliesLoadedPassiveSkills(t *testing.T) {
	skills := modelskill.NewTable([]modelskill.Definition{
		{ID: 200, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 12},
		}},
		{ID: 201, Level: 1, Activation: modelskill.ActivationToggle},
	})
	tmpl := &item.Template{
		ID: 103,
		AttachedSkills: []item.SkillRef{
			{ID: 200, Level: 1}, // passive: contributes
			{ID: 201, Level: 1}, // not passive: skipped
			{ID: 999, Level: 1}, // unloaded: skipped
		},
	}
	owner := ItemOwner{Inst: &item.Instance{ObjectID: 1, TemplateID: 103}, Tmpl: tmpl}

	fns, err := ItemPassiveFuncs(skills, owner)
	if err != nil {
		t.Fatalf("ItemPassiveFuncs() error: %v", err)
	}
	if len(fns) != 1 {
		t.Fatalf("len(fns) = %d, want 1", len(fns))
	}
	if fns[0].Owner != ModOwnerItem(owner) {
		t.Fatalf("Owner = %v, want %v", fns[0].Owner, ModOwnerItem(owner))
	}
}

// ---- from list_activebyskillid_test.go ----
func TestListActiveBySkillIDFindsActiveEffectLevel(t *testing.T) {
	list := NewList(nil)
	list.Add(&Effect{Skill: Skill{ID: 1285}, Level: 3, Type: TypeBuff})

	level, ok := list.ActiveBySkillID(1285)
	if !ok || level != 3 {
		t.Fatalf("ActiveBySkillID(1285) = (%d, %v), want (3, true)", level, ok)
	}
}

func TestListActiveBySkillIDMissReportsNotFound(t *testing.T) {
	list := NewList(nil)
	list.Add(&Effect{Skill: Skill{ID: 1285}, Level: 3, Type: TypeBuff})

	level, ok := list.ActiveBySkillID(5104)
	if ok || level != 0 {
		t.Fatalf("ActiveBySkillID(5104) = (%d, %v), want (0, false)", level, ok)
	}
}

func TestListActiveBySkillIDIgnoresRemovedEffect(t *testing.T) {
	list := NewList(nil)
	e := &Effect{Skill: Skill{ID: 5104}, Level: 2, Type: TypeBuff}
	list.Add(e)
	list.Remove(e)

	if level, ok := list.ActiveBySkillID(5104); ok {
		t.Fatalf("ActiveBySkillID(5104) after Remove = (%d, %v), want ok=false", level, ok)
	}
}

func TestListActiveBySkillIDNilListReportsNotFound(t *testing.T) {
	var list *List
	if level, ok := list.ActiveBySkillID(5104); ok || level != 0 {
		t.Fatalf("nil list ActiveBySkillID = (%d, %v), want (0, false)", level, ok)
	}
}

// ---- from list_test.go ----
type eventOwner struct {
	events  *[]string
	maxBuff int
}

func (o eventOwner) AddStatFuncs([]Mod) {
	*o.events = append(*o.events, "owner:add")
}

func (o eventOwner) RemoveStatsByOwner(owner ModOwner) {
	e := owner.effect
	*o.events = append(*o.events, "owner:remove:"+e.Template.Name)
}

func (o eventOwner) MaxBuffCount() int {
	if o.maxBuff == 0 {
		return 20
	}
	return o.maxBuff
}

func (o eventOwner) NotifyEffectWornOff(skillID modelskill.ID, level int) {
	*o.events = append(*o.events, fmt.Sprintf("worn-off:%d:%d", skillID, level))
}

func (o eventOwner) NotifyEffectDisappeared(skillID modelskill.ID, level int) {
	*o.events = append(*o.events, fmt.Sprintf("disappeared:%d:%d", skillID, level))
}

func (o eventOwner) NotifyEffectAborted(skillID modelskill.ID, level int) {
	*o.events = append(*o.events, fmt.Sprintf("aborted:%d:%d", skillID, level))
}

func newEffect(name string, id modelskill.ID, stackType string, stackOrder float64, debuff bool) *Effect {
	e := &Effect{
		Skill: Skill{
			ID:     id,
			Debuff: debuff,
		},
		Template: modelskill.EffectTemplate{
			Name:       name,
			StackType:  stackType,
			StackOrder: stackOrder,
		},
		Type: TypeBuff,
	}
	e.OnStart = func(*Effect) bool {
		e.Template.Value++
		return true
	}
	return e
}

// flagGatedEffect returns a named debuff carrying flag and marked
// RejectsIfAffected, matching how New() builds a Stun/Root/Sleep/Fear
// effect: it must never be added while the owner already carries flag from
// any currently held effect.
func flagGatedEffect(name string, id modelskill.ID, flag Flag, events *[]string) *Effect {
	e := namedEffect(name, id, "none", 0, true, events)
	e.Flag = flag
	e.RejectsIfAffected = true
	return e
}

func namedEffect(name string, id modelskill.ID, stackType string, stackOrder float64, debuff bool, events *[]string) *Effect {
	e := newEffect(name, id, stackType, stackOrder, debuff)
	e.OnStart = func(*Effect) bool {
		*events = append(*events, name+":start")
		return true
	}
	e.OnExit = func(*Effect) {
		*events = append(*events, name+":exit")
	}
	e.OnStopTask = func(*Effect) {
		*events = append(*events, name+":stop")
	}
	return e
}

func effectNames(effects []*Effect) []string {
	names := make([]string, len(effects))
	for i, e := range effects {
		names[i] = e.Template.Name
	}
	return names
}

func requireEvents(t *testing.T, got []string, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

func requireNames(t *testing.T, got []*Effect, want []string) {
	t.Helper()
	if names := effectNames(got); !reflect.DeepEqual(names, want) {
		t.Fatalf("effects = %#v, want %#v", names, want)
	}
}

// The event and ordering expectations below were generated with a Java probe
// that keeps EffectList's stack insertion, replacement, and activation
// branches intact while replacing actor/network dependencies with log hooks.

func TestListReplacesLowerOrderStackedEffect(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events})

	weak := namedEffect("weak", 1, "speed", 1, false, &events)
	strong := namedEffect("strong", 2, "speed", 2, false, &events)

	list.Add(weak)
	list.Add(strong)

	requireEvents(t, events, []string{
		"weak:start",
		"owner:add",
		"owner:remove:weak",
		"weak:exit",
		"strong:start",
		"owner:add",
	})
	if weak.InUse() {
		t.Fatal("weaker effect stayed active after stronger replacement")
	}
	if !strong.InUse() {
		t.Fatal("stronger effect is not active")
	}
	requireNames(t, list.All(), []string{"strong"})
}

func TestListReactivatesNextStackedEffectWhenCancellationDisabled(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events}, WithCancelLesser(false))

	weak := namedEffect("weak", 1, "speed", 1, false, &events)
	strong := namedEffect("strong", 2, "speed", 2, false, &events)

	list.Add(weak)
	list.Add(strong)
	list.Remove(strong)

	requireEvents(t, events, []string{
		"weak:start",
		"owner:add",
		"owner:remove:weak",
		"weak:exit",
		"strong:start",
		"owner:add",
		"owner:remove:strong",
		"strong:exit",
		"weak:start",
		"owner:add",
	})
	if !weak.InUse() {
		t.Fatal("next stacked effect was not reactivated")
	}
	if strong.InUse() {
		t.Fatal("removed effect stayed active")
	}
	requireNames(t, list.All(), []string{"weak"})
}

func TestListOrdersBuffsBeforeTogglesThenDebuffs(t *testing.T) {
	list := NewList(nil)
	first := newEffect("first", 1, "none", 0, false)
	toggle := newEffect("toggle", 2, "none", 0, false)
	toggle.Skill.Toggle = true
	second := newEffect("second", 3, "none", 0, false)
	debuff := newEffect("debuff", 4, "none", 0, true)
	debuff.Type = TypeDebuff

	list.Add(first)
	list.Add(toggle)
	list.Add(second)
	list.Add(debuff)

	requireNames(t, list.All(), []string{"first", "second", "toggle", "debuff"})
}

func TestListDanceCountCountsOnlyActiveDanceSkillToggles(t *testing.T) {
	list := NewList(nil)

	dance1 := newEffect("dance1", 1, "none", 0, false)
	dance1.Skill.Toggle = true
	dance1.Skill.Dance = true
	list.Add(dance1)

	dance2 := newEffect("dance2", 2, "none", 0, false)
	dance2.Skill.Toggle = true
	dance2.Skill.Dance = true
	list.Add(dance2)

	buff := newEffect("buff", 3, "none", 0, false)
	list.Add(buff)

	if got := list.DanceCount(); got != 2 {
		t.Fatalf("DanceCount() = %d, want 2 (buff and pending effects must not count)", got)
	}

	list.Remove(dance1)
	if got := list.DanceCount(); got != 1 {
		t.Fatalf("DanceCount() after removing one dance = %d, want 1", got)
	}
}

func TestListReplacesIdenticalBuffButRejectsIdenticalDebuff(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events})

	buff1 := namedEffect("buff1", 1, "none", 0, false, &events)
	buff2 := namedEffect("buff2", 1, "none", 0, false, &events)
	debuff1 := namedEffect("debuff1", 2, "hex", 3, true, &events)
	debuff1.Type = TypeDebuff
	debuff2 := namedEffect("debuff2", 2, "hex", 3, true, &events)
	debuff2.Type = TypeDebuff

	list.Add(buff1)
	list.Add(buff2)
	list.Add(debuff1)
	list.Add(debuff2)

	requireEvents(t, events, []string{
		"buff1:start",
		"owner:add",
		"buff1:stop",
		"owner:remove:buff1",
		"buff1:exit",
		"buff2:start",
		"owner:add",
		"debuff1:start",
		"owner:add",
		"debuff2:stop",
	})
	requireNames(t, list.All(), []string{"buff2", "debuff1"})
	if !buff2.InUse() || !debuff1.InUse() {
		t.Fatal("replacement effects are not active")
	}
	if debuff2.InUse() {
		t.Fatal("rejected identical debuff became active")
	}
}

// buffSlotEffect returns a named, non-stacking buff-slot-family effect
// (the family of skill types that occupy an owner's limited buff slots and
// are shown as an icon).
func buffSlotEffect(name string, id modelskill.ID, events *[]string) *Effect {
	e := namedEffect(name, id, "none", 0, false, events)
	e.Skill.SkillType = "BUFF"
	e.Template.Icon = true
	return e
}

func TestListEvictsOldestBuffSlotEffectAtCapacity(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events, maxBuff: 2})

	first := buffSlotEffect("first", 1, &events)
	second := buffSlotEffect("second", 2, &events)
	third := buffSlotEffect("third", 3, &events)

	list.Add(first)
	list.Add(second)
	list.Add(third)

	requireNames(t, list.All(), []string{"second", "third"})
	if first.InUse() {
		t.Fatal("evicted buff stayed active")
	}
	found := false
	for _, ev := range events {
		if ev == "first:stop" {
			found = true
		}
	}
	if !found {
		t.Fatal("evicted buff's task was never stopped")
	}
}

func TestListDropsHerbEffectAtCapacityWithoutEvicting(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events, maxBuff: 1})

	real := buffSlotEffect("real", 1, &events)
	herb := namedEffect("herb", 2, "none", 0, false, &events)
	herb.Herb = true

	list.Add(real)
	list.Add(herb)

	requireEvents(t, events, []string{
		"real:start",
		"owner:add",
		"herb:stop",
	})
	requireNames(t, list.All(), []string{"real"})
	if herb.InUse() {
		t.Fatal("dropped herb effect became active")
	}
}

func TestListSkipsCapEvictionForIncomingStackingBuff(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events, maxBuff: 2})

	unrelated := buffSlotEffect("unrelated", 1, &events)
	weak := namedEffect("weak", 2, "speed", 1, false, &events)
	weak.Skill.SkillType = "BUFF"
	weak.Template.Icon = true
	strong := namedEffect("strong", 3, "speed", 2, false, &events)
	strong.Skill.SkillType = "BUFF"
	strong.Template.Icon = true

	list.Add(unrelated)
	list.Add(weak)
	list.Add(strong)

	requireNames(t, list.All(), []string{"unrelated", "strong"})
	if !unrelated.InUse() {
		t.Fatal("unrelated buff was displaced by cap eviction instead of the stack-group replacement")
	}
	if weak.InUse() {
		t.Fatal("weaker stacked buff stayed active")
	}
	if !strong.InUse() {
		t.Fatal("stronger stacked buff is not active")
	}
}

// runWithDeadlockGuard runs fn and fails t if it doesn't return within a
// short timeout, the symptom a reentrant hook self-deadlocking on List.mu
// would produce.
func runWithDeadlockGuard(t *testing.T, name string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s deadlocked", name)
	}
}

func TestListOnStartHookCanReenterAddWithoutDeadlock(t *testing.T) {
	list := NewList(nil)
	followUp := newEffect("followup", 2, "none", 0, false)

	reentrant := newEffect("reentrant", 1, "none", 0, false)
	reentrant.OnStart = func(*Effect) bool {
		list.Add(followUp)
		return true
	}

	runWithDeadlockGuard(t, "List.Add", func() {
		list.Add(reentrant)
	})

	requireNames(t, list.All(), []string{"reentrant", "followup"})
	if !followUp.InUse() {
		t.Fatal("effect added from within a reentrant OnStart hook never activated")
	}
}

func TestListOnExitHookCanReenterAddWithoutDeadlock(t *testing.T) {
	list := NewList(nil)
	var followUp *Effect

	reentrant := newEffect("reentrant", 1, "none", 0, false)
	reentrant.OnExit = func(*Effect) {
		followUp = newEffect("followup", 2, "none", 0, false)
		list.Add(followUp)
	}
	list.Add(reentrant)

	runWithDeadlockGuard(t, "List.Remove", func() {
		list.Remove(reentrant)
	})

	if followUp == nil || !followUp.InUse() {
		t.Fatal("effect added from within a reentrant OnExit hook never activated")
	}
}

func TestListFlagsAggregatesActiveEffectFlagsAndDropsThemOnRemoval(t *testing.T) {
	list := NewList(nil)

	stun := newEffect("stun", 1, "none", 0, true)
	stun.Flag = FlagStunned
	root := newEffect("root", 2, "none", 0, true)
	root.Flag = FlagRooted
	fear := newEffect("fear", 3, "none", 0, true)
	fear.Flag = FlagFear
	paralyze := newEffect("paralyze", 4, "none", 0, true)
	paralyze.Flag = FlagParalyzed

	if got := list.Flags(); got != 0 {
		t.Fatalf("Flags() on an empty list = %#x, want 0", got)
	}

	list.Add(stun)
	if !list.IsAffected(FlagStunned) {
		t.Fatal("IsAffected(FlagStunned) = false after adding a stun effect")
	}
	if list.IsAffected(FlagRooted) || list.IsAffected(FlagFear) || list.IsAffected(FlagParalyzed) {
		t.Fatal("IsAffected reported a flag from an effect never added")
	}

	list.Add(root)
	list.Add(fear)
	list.Add(paralyze)

	for _, flag := range []Flag{FlagStunned, FlagRooted, FlagFear, FlagParalyzed} {
		if !list.IsAffected(flag) {
			t.Fatalf("IsAffected(%#x) = false, want true with all four effects active", flag)
		}
	}
	if want := FlagStunned | FlagRooted | FlagFear | FlagParalyzed; list.Flags() != want {
		t.Fatalf("Flags() = %#x, want %#x", list.Flags(), want)
	}

	list.Remove(stun)
	if list.IsAffected(FlagStunned) {
		t.Fatal("IsAffected(FlagStunned) still true after its effect was removed")
	}
	if !list.IsAffected(FlagRooted) || !list.IsAffected(FlagFear) || !list.IsAffected(FlagParalyzed) {
		t.Fatal("removing one flagged effect cleared an unrelated flag")
	}

	list.Remove(root)
	list.Remove(fear)
	list.Remove(paralyze)
	if got := list.Flags(); got != 0 {
		t.Fatalf("Flags() after removing every effect = %#x, want 0", got)
	}
}

func TestListRejectsSecondFlagGatedEffectOfEachKindWhileFirstIsActive(t *testing.T) {
	tests := []struct {
		name string
		flag Flag
	}{
		{"stun", FlagStunned},
		{"root", FlagRooted},
		{"sleep", FlagSleep},
		{"fear", FlagFear},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var events []string
			list := NewList(eventOwner{events: &events})

			first := flagGatedEffect(tt.name+"1", 1, tt.flag, &events)
			second := flagGatedEffect(tt.name+"2", 2, tt.flag, &events)

			list.Add(first)
			list.Add(second)

			requireEvents(t, events, []string{
				tt.name + "1:start",
				"owner:add",
				tt.name + "2:stop",
			})
			if !first.InUse() {
				t.Fatal("first flag-gated effect was displaced instead of the second one being rejected")
			}
			if second.InUse() {
				t.Fatal("second flag-gated effect was added while its own flag was already active")
			}
			requireNames(t, list.All(), []string{tt.name + "1"})
		})
	}
}

func TestListRejectsFlagGatedEffectWhenFlagIsSetByADifferentEffectKind(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events})

	// stunSelf carries FlagStunned but, like the reference StunSelf effect,
	// is not itself flag-gated: it only ever blocks other Stunned-flag
	// effects, it never rejects itself.
	stunSelf := namedEffect("stunself", 1, "none", 0, false, &events)
	stunSelf.Flag = FlagStunned

	stun := flagGatedEffect("stun", 2, FlagStunned, &events)

	list.Add(stunSelf)
	list.Add(stun)

	requireEvents(t, events, []string{
		"stunself:start",
		"owner:add",
		"stun:stop",
	})
	if !stunSelf.InUse() {
		t.Fatal("stunself was displaced instead of the incoming stun being rejected")
	}
	if stun.InUse() {
		t.Fatal("stun was added despite FlagStunned already being set by a different effect kind")
	}
	requireNames(t, list.All(), []string{"stunself"})
}

func TestListDoesNotFlagGateParalyzeOrPetrificationEffects(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events})

	// Paralyze and Petrification both carry FlagParalyzed but, unlike
	// Stun/Root/Sleep/Fear, neither is flag-gated: a second one proceeds
	// through the ordinary buff/debuff handling instead of being rejected
	// outright.
	paralyze1 := namedEffect("paralyze1", 1, "none", 0, true, &events)
	paralyze1.Flag = FlagParalyzed
	paralyze2 := namedEffect("paralyze2", 2, "none", 0, true, &events)
	paralyze2.Flag = FlagParalyzed
	petrification := namedEffect("petrification", 3, "none", 0, true, &events)
	petrification.Flag = FlagParalyzed

	list.Add(paralyze1)
	list.Add(paralyze2)
	list.Add(petrification)

	if !paralyze1.InUse() {
		t.Fatal("first paralyze effect is not active")
	}
	if !paralyze2.InUse() {
		t.Fatal("second paralyze effect was rejected despite FlagParalyzed not being flag-gated")
	}
	if !petrification.InUse() {
		t.Fatal("petrification was rejected despite FlagParalyzed not being flag-gated")
	}
	requireNames(t, list.All(), []string{"paralyze1", "paralyze2", "petrification"})
}

func TestListRejectsSameSkillRecastOfFlagGatedEffectBeforeIdenticalDebuffLogic(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events})

	first := flagGatedEffect("stun", 7, FlagStunned, &events)
	recast := flagGatedEffect("stun", 7, FlagStunned, &events)
	// Line up every field the identical-debuff-reject branch compares, so
	// that branch alone (absent the flag gate) would produce the very same
	// "reject the incoming effect" outcome. The flag gate must still be
	// what actually fires: it runs before that branch is ever reached, and
	// it rejects based on the flag alone, not a same-skill/same-stack match.
	recast.Type = first.Type
	recast.Template.StackOrder = first.Template.StackOrder
	recast.Template.StackType = first.Template.StackType

	list.Add(first)
	list.Add(recast)

	requireEvents(t, events, []string{
		"stun:start",
		"owner:add",
		"stun:stop",
	})
	if !first.InUse() {
		t.Fatal("original stun was removed/exited instead of the recast being rejected")
	}
	if recast.InUse() {
		t.Fatal("recast stun became active")
	}
	requireNames(t, list.All(), []string{"stun"})
}

// abnormalUpdateOwner is a StatOwner that also implements abnormalUpdater,
// recording one call per notification.
type abnormalUpdateOwner struct {
	eventOwner
	calls *int
}

func (o abnormalUpdateOwner) UpdateAbnormalEffect() {
	*o.calls++
}

func TestListNotifiesAbnormalUpdateOnEveryAddAndRemove(t *testing.T) {
	var events []string
	var calls int
	list := NewList(abnormalUpdateOwner{eventOwner: eventOwner{events: &events}, calls: &calls})

	e := namedEffect("buff", 1, "none", 0, false, &events)
	list.Add(e)
	if calls != 1 {
		t.Fatalf("calls after Add = %d, want 1", calls)
	}

	list.Remove(e)
	if calls != 2 {
		t.Fatalf("calls after Remove = %d, want 2", calls)
	}
}

func TestListIconEntriesSkipsEffectsWithoutShowIconOrNotActive(t *testing.T) {
	list := NewList(nil)

	shown := &Effect{
		Skill:    Skill{ID: 10, Level: 3},
		Template: modelskill.EffectTemplate{Name: "buff", Time: -1, Icon: true},
		Level:    3,
	}
	shown.OnStart = func(*Effect) bool { return true }

	hidden := &Effect{
		Skill:    Skill{ID: 11, Level: 1},
		Template: modelskill.EffectTemplate{Name: "buff", Time: -1, Icon: false},
	}
	hidden.OnStart = func(*Effect) bool { return true }

	signetGround := &Effect{
		Skill:    Skill{ID: 12, Level: 1},
		Template: modelskill.EffectTemplate{Name: "buff", Time: -1, Icon: true, EffectType: "SIGNET_GROUND"},
	}
	signetGround.OnStart = func(*Effect) bool { return true }

	list.Add(shown)
	list.Add(hidden)
	list.Add(signetGround)

	entries := list.IconEntries(time.Now())
	if len(entries) != 1 || entries[0].ID != 10 || entries[0].Level != 3 || entries[0].Duration != -1 {
		t.Fatalf("IconEntries() = %+v, want one permanent entry for skill 10", entries)
	}
}

// TestListIconEntriesSeedShowsSkillLevelNotGrownPower guards
// AbnormalStatusUpdate.addEffect(skill, ...) -> EffectHolder(skill, period)
// (EffectHolder.java), which always sends skill.getLevel() for the icon —
// never EffectSeed._power. A seed's Level field doubles as its charge
// counter (grown by IncreasePower), so the icon must read Skill.Level
// instead of the grown Level.
func TestListIconEntriesSeedShowsSkillLevelNotGrownPower(t *testing.T) {
	list := NewList(nil)

	seed := &Effect{
		Skill:    Skill{ID: 1285, Level: 1},
		Type:     TypeSeed,
		Template: modelskill.EffectTemplate{Name: "Seed", Time: 5, Icon: true},
		Level:    4, // grown via three IncreasePower() calls
	}
	seed.OnStart = func(*Effect) bool { return true }
	list.Add(seed)

	entries := list.IconEntries(time.Now())
	if len(entries) != 1 || entries[0].Level != 1 {
		t.Fatalf("IconEntries() = %+v, want skill level 1, not grown power 4", entries)
	}
}

func TestListIconEntriesReportsToggleAndRepeatCountDurations(t *testing.T) {
	list := NewList(nil)

	toggle := &Effect{
		Skill:    Skill{ID: 20, Level: 1, Toggle: true},
		Template: modelskill.EffectTemplate{Name: "buff", Time: -1, Icon: true},
	}
	toggle.OnStart = func(*Effect) bool { return true }

	repeat := &Effect{
		Skill:    Skill{ID: 21, Level: 1},
		Template: modelskill.EffectTemplate{Name: "dot", Time: 2, Count: 5, Icon: true},
	}
	repeat.OnStart = func(*Effect) bool { return true }

	list.Add(toggle)
	list.Add(repeat)

	entries := list.IconEntries(time.Now())
	if len(entries) != 2 {
		t.Fatalf("IconEntries() len = %d, want 2", len(entries))
	}
	// insertBuff always places a non-toggle buff ahead of any toggle
	// already in the list, so the repeat-count entry (non-toggle) comes
	// first even though the toggle was added first.
	if entries[0].ID != 21 || entries[0].Toggle || entries[0].Duration != 10_000 {
		t.Fatalf("repeat-count entry = %+v, want id 21, non-toggle, duration 10000ms (5*2s)", entries[0])
	}
	if entries[1].ID != 20 || !entries[1].Toggle || entries[1].Duration != -1 {
		t.Fatalf("toggle entry = %+v, want id 20, toggle, duration -1", entries[1])
	}
}

// TestIconDurationRepeatCountDecrementsEverySecond is the count=7/time=2 HOT
// case from AbstractEffect.addIcon's repeat-count branch (AbstractEffect.java
// :340-353): (getCounter()*_period - getTaskTime()) * 1000, where
// getTaskTime() (AbstractEffect.java:146-152) is elapsed whole seconds since
// the current tick started. The reference counts down 14,13,12,...,1 —
// one second at a time — never holding flat for a whole 2s tick window the
// way Remaining()*Template.Time*1000 alone would.
func TestIconDurationRepeatCountDecrementsEverySecond(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Name: "dot", Time: 2, Count: 7, Icon: true}}
	start := time.Unix(0, 0)
	e.startSchedule(start)

	cases := []struct {
		offset time.Duration
		want   int32
	}{
		{0, 14000},
		{1 * time.Second, 13000},
		{2 * time.Second, 12000}, // first tick claimed exactly here, below
	}
	for _, c := range cases[:2] {
		if got, ok := e.iconDuration(start.Add(c.offset)); !ok || got != c.want {
			t.Fatalf("iconDuration(+%s) = %d, %v; want %d, true", c.offset, got, ok, c.want)
		}
	}

	if run, remove := e.claimAction(start.Add(2 * time.Second)); !run || remove {
		t.Fatalf("claimAction at 2s = run %v remove %v, want run=true remove=false (6 ticks remain)", run, remove)
	}
	if got, ok := e.iconDuration(start.Add(2 * time.Second)); !ok || got != 12000 {
		t.Fatalf("iconDuration(+2s, post-tick) = %d, %v; want 12000, true", got, ok)
	}
	if got, ok := e.iconDuration(start.Add(3 * time.Second)); !ok || got != 11000 {
		t.Fatalf("iconDuration(+3s) = %d, %v; want 11000, true (old formula held flat at 12000 here)", got, ok)
	}
}

// TestListDisplacedStackedEffectDrainsCountWithoutActingAndResumesWithoutRestartOnPromotion
// proves a stacked-out loser's tick schedule keeps draining while displaced —
// mirroring AbstractEffect.scheduleEffect()'s ACTING case, which decrements
// _count on every tick regardless of getInUse() and only runs
// onActionTime() when getInUse() is true (AbstractEffect.java:291-306) — and
// that a promoted loser resumes from that drained count instead of
// restarting from the template.
func TestListDisplacedStackedEffectDrainsCountWithoutActingAndResumesWithoutRestartOnPromotion(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events}, WithCancelLesser(false))

	weak := namedEffect("weak", 1, "speed", 1, false, &events)
	weak.Template.Count, weak.Template.Time = 5, 2
	weak.OnAction = func(*Effect) bool { events = append(events, "weak:action"); return true }

	strong := namedEffect("strong", 2, "speed", 2, false, &events)
	strong.Template.Count, strong.Template.Time = 1, 2
	strong.OnAction = func(*Effect) bool { events = append(events, "strong:action"); return true }

	list.Add(weak)
	start := time.Unix(1000, 0)
	weak.startSchedule(start) // pin weak's schedule to a deterministic clock

	list.Add(strong)
	strong.startSchedule(start) // strong displaces weak; pin its schedule too

	// weak is buffs[0] (added first), so tickAt claims weak's action before
	// strong's on every sweep — see List.All()'s buffs-then-debuffs order.
	// strong's count (1) exhausts on this same tick, so it is removed and
	// weak is promoted in its place within the same tickAt call.
	list.tickAt(start.Add(2 * time.Second))

	requireNames(t, list.All(), []string{"weak"})
	if !weak.InUse() {
		t.Fatal("weak was not promoted after strong (its only stack winner) was removed")
	}
	if weak.Remaining() != 4 {
		t.Fatalf("weak.Remaining() after promotion = %d, want 4 (resumed from the drained count, not restarted to template count 5)", weak.Remaining())
	}
	for _, e := range events {
		if e == "weak:action" {
			t.Fatal("weak fired its periodic action while still displaced")
		}
	}
}

// TestListDisplacedStackedEffectSelfRemovesOnCountExhaustionWithoutEverActivating
// proves a stacked-out loser that never gets promoted still self-removes once
// its own count drains to zero, mirroring scheduleEffect()'s ACTING case
// falling through to FINISHING (and stopEffectTask() removing the effect)
// once _count reaches 0, regardless of getInUse() (AbstractEffect.java:
// 291-313).
func TestListDisplacedStackedEffectSelfRemovesOnCountExhaustionWithoutEverActivating(t *testing.T) {
	var events []string
	list := NewList(eventOwner{events: &events}, WithCancelLesser(false))

	weak := namedEffect("weak", 1, "speed", 1, false, &events)
	weak.Template.Count, weak.Template.Time = 2, 2
	weak.OnAction = func(*Effect) bool { events = append(events, "weak:action"); return true }

	strong := namedEffect("strong", 2, "speed", 2, false, &events)
	strong.Template.Count, strong.Template.Time = 100, 2
	strong.OnAction = func(*Effect) bool { events = append(events, "strong:action"); return true }

	list.Add(weak)
	start := time.Unix(2000, 0)
	weak.startSchedule(start)

	list.Add(strong)
	strong.startSchedule(start)

	list.tickAt(start.Add(2 * time.Second))
	if weak.Remaining() != 1 {
		t.Fatalf("weak.Remaining() after one tick = %d, want 1", weak.Remaining())
	}
	requireNames(t, list.All(), []string{"weak", "strong"})

	list.tickAt(start.Add(4 * time.Second))
	requireNames(t, list.All(), []string{"strong"})
	if weak.InUse() {
		t.Fatal("weak reports in-use after self-removing while displaced")
	}
	for _, e := range events {
		if e == "weak:action" {
			t.Fatal("weak fired its periodic action even though it was never promoted")
		}
	}
}

// TestListEffectExpiryMessages proves each of the three system-message
// variants EffectList.removeEffectFromQueue sends fires for its
// corresponding removal path (EffectList.java:572-584): natural count
// exhaustion (worn off), early removal (disappeared), and a toggle skill
// turned off (aborted).
func TestListEffectExpiryMessages(t *testing.T) {
	t.Run("worn off on count exhaustion", func(t *testing.T) {
		var events []string
		list := NewList(eventOwner{events: &events})
		e := namedEffect("wornoff", 10, "none", 0, false, &events)
		e.Skill.Level = 3
		e.Template.Count, e.Template.Time, e.Template.Icon = 1, 1, true
		list.Add(e)
		start := time.Unix(1000, 0)
		e.startSchedule(start)

		list.tickAt(start.Add(1 * time.Second))

		if !slices.Contains(events, "worn-off:10:3") {
			t.Fatalf("events = %v, want worn-off:10:3", events)
		}
	})

	t.Run("disappeared on early removal", func(t *testing.T) {
		var events []string
		list := NewList(eventOwner{events: &events})
		e := namedEffect("early", 11, "none", 0, false, &events)
		e.Skill.Level = 2
		e.Template.Count, e.Template.Time, e.Template.Icon = 5, 1, true
		list.Add(e)

		list.Remove(e)

		if !slices.Contains(events, "disappeared:11:2") {
			t.Fatalf("events = %v, want disappeared:11:2", events)
		}
	})

	t.Run("aborted on toggle turned off", func(t *testing.T) {
		var events []string
		list := NewList(eventOwner{events: &events})
		e := namedEffect("toggle", 12, "none", 0, false, &events)
		e.Skill.Level = 1
		e.Skill.Toggle = true
		e.Template.Icon = true
		list.Add(e)

		list.Remove(e)

		if !slices.Contains(events, "aborted:12:1") {
			t.Fatalf("events = %v, want aborted:12:1", events)
		}
	})

	t.Run("no message without icon", func(t *testing.T) {
		var events []string
		list := NewList(eventOwner{events: &events})
		e := namedEffect("noicon", 13, "none", 0, false, &events)
		list.Add(e)

		list.Remove(e)

		for _, ev := range events {
			if strings.HasPrefix(ev, "worn-off:") || strings.HasPrefix(ev, "disappeared:") || strings.HasPrefix(ev, "aborted:") {
				t.Fatalf("events = %v, want no expiry message for a non-icon effect", events)
			}
		}
	})
}

func TestListRemoveStackedEffectWithoutQueueLeavesVisible(t *testing.T) {
	list := NewList(nil)
	e := newEffect("stacked", 1, "speed", 1, false)
	list.Add(e)

	delete(list.stacks, "speed")
	list.Remove(e)

	requireNames(t, list.All(), []string{"stacked"})
}

func TestListRemoveStackedEffectAbsentFromQueueRemovesVisible(t *testing.T) {
	list := NewList(nil, WithCancelLesser(false))
	removed := newEffect("removed", 1, "speed", 1, false)
	remaining := newEffect("remaining", 2, "speed", 2, false)
	list.Add(removed)
	list.Add(remaining)

	list.stacks["speed"] = []*Effect{remaining}
	list.Remove(removed)

	requireNames(t, list.All(), []string{"remaining"})
}

// ---- from mod_test.go ----
func TestCalculatorOrdering(t *testing.T) {
	var c Calculator

	// Attach out of order; AddMod must still run them low-order-first.
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpAdd, Value: 5})     // order 30
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpMul, Value: 2})     // order 20
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpBaseAdd, Value: 3}) // order 2

	// base=10: BaseAdd -> 13, Mul -> 26, Add -> 31.
	got := c.Calc(nil, 10)
	if got != 31 {
		t.Errorf("Calc() = %v, want 31", got)
	}
	if c.Size() != 3 {
		t.Errorf("Size() = %d, want 3", c.Size())
	}
}

func TestCalculatorSetOverridesBase(t *testing.T) {
	var c Calculator
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpSet, Value: 100})
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpBaseMul, Value: 0.1})

	// Set (order 0) runs first, replacing base with 100 (value=100). Then
	// BaseMul (order 1) adds base*0.1 = 100*0.1 = 10 to the running value:
	// 100 + 10 = 110.
	got := c.Calc(nil, 5)
	if got != 110 {
		t.Errorf("Calc() = %v, want 110", got)
	}
}

func TestCalculatorRemoveOwner(t *testing.T) {
	var c Calculator
	ownerA := ModOwnerEffect(&Effect{})
	ownerB := ModOwnerEffect(&Effect{})
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpAdd, Value: 5, Owner: ownerA})
	c.AddMod(Mod{Stat: stat.MagicAttack, Op: OpAdd, Value: 7, Owner: ownerB})
	c.AddMod(Mod{Stat: stat.CriticalRate, Op: OpAdd, Value: 1, Owner: ownerA})

	c.RemoveOwner(ownerA)
	if c.Size() != 1 {
		t.Fatalf("Size() = %d, want 1", c.Size())
	}
	if got := c.Calc(nil, 0); got != 7 {
		t.Errorf("Calc() = %v, want 7 (only ownerB's mod left)", got)
	}
}

func TestCalculatorEmpty(t *testing.T) {
	var c Calculator
	if got := c.Calc(nil, 42); got != 42 {
		t.Errorf("Calc() on empty chain = %v, want base unchanged 42", got)
	}
}

func TestCalculatorBuiltinRunsBetweenLowAndHighOrderMods(t *testing.T) {
	c := NewCalculator(func(actor stat.Actor, base, value float64) float64 {
		return value + 1000
	})
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpBaseAdd, Value: 3}) // order 2, before builtin
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpAdd, Value: 5})     // order 30, after builtin

	// base=10: BaseAdd -> 13, builtin -> 1013, Add -> 1018.
	if got := c.Calc(nil, 10); got != 1018 {
		t.Errorf("Calc() = %v, want 1018", got)
	}
}

// TestCalculatorConcurrentReadsAndMutationsAreRace-Free exercises the
// concurrency guarantee #1527 replaced: many goroutines call Calc
// concurrently with other goroutines attaching and detaching Mods. This
// test's only purpose is to be run under -race; it makes no assertion
// about the numeric outcome, which is inherently nondeterministic under
// concurrent mutation.
func TestCalculatorConcurrentReadsAndMutationsAreRaceFree(t *testing.T) {
	var c Calculator
	owner := ModOwnerEffect(&Effect{})

	var readers, writers sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					c.Calc(nil, 10)
				}
			}
		}()
	}

	for i := 0; i < 2; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for j := 0; j < 200; j++ {
				c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpAdd, Value: 1, Owner: owner})
				c.RemoveOwner(owner)
			}
		}()
	}

	writers.Wait()
	close(stop)
	readers.Wait()
}

// ---- from passive_test.go ----
// The skill id, level, and Funcs below reproduce the "Toughness" passive
// (skill 134): a flat 20% vulnerability increase to three abnormal
// resistances, carried as top-level Funcs rather than an effect template.

func TestPassiveFuncsBuildsFuncsOwnedByTheSkillLevel(t *testing.T) {
	def := modelskill.Definition{
		ID:         134,
		Level:      1,
		Activation: modelskill.ActivationPassive,
		Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAddMul, Stat: "rootVuln", Value: 20},
			{Op: modelskill.FuncAddMul, Stat: "sleepVuln", Value: 20},
			{Op: modelskill.FuncAddMul, Stat: "poisonVuln", Value: 20},
		},
	}

	funcs, err := PassiveFuncs(def)
	if err != nil {
		t.Fatalf("PassiveFuncs() error: %v", err)
	}
	if len(funcs) != 3 {
		t.Fatalf("Funcs length = %d, want 3", len(funcs))
	}

	wantOwner := ModOwnerSkill(modelskill.Ref{ID: 134, Level: 1})
	for i, fn := range funcs {
		if fn.Owner != wantOwner {
			t.Fatalf("funcs[%d].Owner = %v, want %v", i, fn.Owner, wantOwner)
		}
	}
	if funcs[0].Stat != stat.RootVuln {
		t.Fatalf("funcs[0].Stat = %s, want %s", funcs[0].Stat, stat.RootVuln)
	}
	if got := apply(funcs[0], nil, 100, 100); got != 80 {
		t.Fatalf("apply(funcs[0]) = %v, want 80", got)
	}
}

func TestPassiveFuncsRejectsNonPassiveSkill(t *testing.T) {
	def := modelskill.Definition{ID: 60, Level: 1, Activation: modelskill.ActivationToggle}

	if _, err := PassiveFuncs(def); err == nil {
		t.Fatal("PassiveFuncs() error = nil, want an error for a non-passive skill")
	}
}

func TestPassiveFuncsPropagatesBuildErrors(t *testing.T) {
	def := modelskill.Definition{
		ID:         1,
		Level:      1,
		Activation: modelskill.ActivationPassive,
		Funcs:      []modelskill.FuncTemplate{{Op: modelskill.FuncEnchant, Stat: "pAtk", Value: 1}},
	}

	if _, err := PassiveFuncs(def); err == nil {
		t.Fatal("PassiveFuncs() error = nil, want an error for an ownerless enchant func")
	}
}

func TestSkillStatFuncsIgnoresOperateType(t *testing.T) {
	def := modelskill.Definition{
		ID:         4408,
		Level:      1,
		Activation: modelskill.ActivationActive,
		Funcs:      []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: 250}},
	}

	if _, err := PassiveFuncs(def); err == nil {
		t.Fatal("PassiveFuncs() error = nil, want an error for an active-operate skill")
	}

	funcs, err := SkillStatFuncs(def)
	if err != nil {
		t.Fatalf("SkillStatFuncs() error: %v", err)
	}
	if len(funcs) != 1 {
		t.Fatalf("SkillStatFuncs() length = %d, want 1", len(funcs))
	}
	if funcs[0].Owner != ModOwnerSkill(modelskill.Ref{ID: 4408, Level: 1}) {
		t.Fatalf("Owner = %v, want skill 4408 level 1", funcs[0].Owner)
	}
	if funcs[0].Stat != stat.MaxHP || funcs[0].Op != OpAdd || funcs[0].Value != 250 {
		t.Fatalf("func = %+v, want maxHp add 250", funcs[0])
	}
}

func TestTemplatePassiveModsResolvesRefsAndSkipsMissing(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{{
		ID:         99,
		Level:      1,
		Activation: modelskill.ActivationActive,
		Funcs:      []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: 250}},
	}})

	got := TemplatePassiveMods(table, []modelskill.Ref{
		{ID: 99, Level: 1},
		{ID: 98, Level: 1},
	})
	if len(got) != 1 {
		t.Fatalf("TemplatePassiveMods() length = %d, want 1 (missing ref skipped)", len(got))
	}
	if got[0].Value != 250 {
		t.Fatalf("Value = %v, want 250", got[0].Value)
	}

	if fns := TemplatePassiveMods(nil, []modelskill.Ref{{ID: 99, Level: 1}}); len(fns) != 0 {
		t.Fatalf("nil lookup = %v, want empty", fns)
	}
}

// ---- from persist_restore_test.go ----
func TestSeedRestoreSchedulesFromPersistedCountAndElapsedTime(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 5, Time: 10}}
	e.seedRestore(3, 4) // 3 ticks left, 4s elapsed since the last tick at logout
	e.startSchedule(time.Unix(1000, 0))

	if got := e.Remaining(); got != 3 {
		t.Fatalf("Remaining() = %d, want 3 (persisted count, below template count 5)", got)
	}

	fixedNow := time.Unix(1000, 0)
	if run, _ := e.claimAction(fixedNow.Add(5 * time.Second)); run {
		t.Fatal("claimAction fired before the seeded delay (10-4=6s) elapsed")
	}

	// claimAction above only peeked before the delay elapsed and didn't
	// mutate anything (nextAction is still 6s out), so the same schedule can
	// be claimed again at the 6s mark without reseeding.
	if run, remove := e.claimAction(fixedNow.Add(6 * time.Second)); !run || remove {
		t.Fatalf("claimAction at the seeded delay = run %v remove %v, want run=true remove=false (2 ticks remain)", run, remove)
	}
	if got := e.Remaining(); got != 2 {
		t.Fatalf("Remaining() after first tick = %d, want 2", got)
	}
}

func TestSeedRestoreClampsCountToTemplateCountAndElapsedToPeriod(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 2, Time: 10}}
	e.seedRestore(99, 999) // persisted values exceeding the template's own count/period
	fixedNow := time.Unix(2000, 0)
	e.startSchedule(fixedNow)

	if got := e.Remaining(); got != 2 {
		t.Fatalf("Remaining() = %d, want clamped to template count 2", got)
	}
	if run, _ := e.claimAction(fixedNow); !run {
		t.Fatal("claimAction did not fire immediately when the elapsed time exceeded the period")
	}
}

func TestSeedRestoreNonPeriodicEffectNeverClaims(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 1}}
	e.seedRestore(1, 0)
	e.startSchedule(time.Now())

	if run, remove := e.claimAction(time.Now().Add(time.Hour)); run || remove {
		t.Fatalf("claimAction on a non-periodic restored effect = run %v remove %v, want both false", run, remove)
	}
}

// TestSaveStateIsTheInverseOfSeedRestore proves SaveState and seedRestore
// round-trip: an effect scheduled with a persisted count/elapsed reports
// that same count/elapsed back out once time has actually advanced by the
// elapsed amount, the way a logout mid-period should re-persist the buff's
// remaining state rather than resetting or losing it.
func TestSaveStateIsTheInverseOfSeedRestore(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 5, Time: 10}}
	e.seedRestore(3, 4) // 3 ticks left, 4s elapsed since the last tick at logout
	start := time.Unix(1000, 0)
	e.startSchedule(start)

	count, elapsed := e.SaveState(start)
	if count != 3 || elapsed != 4 {
		t.Fatalf("SaveState() at the seeded instant = (%d, %d), want (3, 4)", count, elapsed)
	}

	count, elapsed = e.SaveState(start.Add(2 * time.Second))
	if count != 3 || elapsed != 6 {
		t.Fatalf("SaveState() 2s later = (%d, %d), want (3, 6)", count, elapsed)
	}
}

func TestSaveStateOnAFreshEffectReportsNoElapsedTime(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 2, Time: 30}}
	now := time.Unix(5000, 0)
	e.startSchedule(now)

	count, elapsed := e.SaveState(now)
	if count != 2 || elapsed != 0 {
		t.Fatalf("SaveState() right after a fresh cast = (%d, %d), want (2, 0)", count, elapsed)
	}
}

// TestSaveStateFloorsSubSecondElapsedInsteadOfRoundingUp guards against
// truncating the remaining-until-next-tick duration to whole seconds before
// subtracting it from the period: doing so floors "remaining" (rounding it
// down) and so rounds the derived elapsed time up, over-reporting by up to a
// full second right after a fresh cast. SaveState must floor the elapsed
// duration itself instead, matching the whole-seconds-so-far semantic
// startScheduleFromRestoreLocked's delay computation expects on restore.
func TestSaveStateFloorsSubSecondElapsedInsteadOfRoundingUp(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 1, Time: 30}}
	start := time.Unix(7000, 0)
	e.startSchedule(start)

	if _, elapsed := e.SaveState(start.Add(time.Millisecond)); elapsed != 0 {
		t.Fatalf("SaveState() 1ms after cast = elapsed %d, want 0 (not rounded up to 1)", elapsed)
	}
	if _, elapsed := e.SaveState(start.Add(29*time.Second + 999*time.Millisecond)); elapsed != 29 {
		t.Fatalf("SaveState() just before the tick = elapsed %d, want 29 (not rounded up to 30, which would restore as an immediate tick)", elapsed)
	}
}

func TestSaveStateOnANonPeriodicEffectReportsNoElapsedTime(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 1}}
	now := time.Unix(6000, 0)
	e.startSchedule(now)

	count, elapsed := e.SaveState(now.Add(time.Hour))
	if count != 1 || elapsed != 0 {
		t.Fatalf("SaveState() on a non-periodic effect = (%d, %d), want (1, 0)", count, elapsed)
	}
}

// TestApplyRestoredDeliversOnStartToLiveEffectList and
// TestApplyRestoredSkipsUnsupportedTemplatesWithoutFailingTheRest moved to
// persist_restore_player_test.go (package effect_test): fakeChargesTarget's
// IncreaseCharges reimplemented the same cap/overflow logic already on the
// real (*player.Character).IncreaseCharges. See docs/agents/test-strategy.md.

// ---- from persist_test.go ----
func ref(id int32, level int) modelskill.Ref {
	return modelskill.Ref{ID: modelskill.ID(id), Level: level}
}

func TestBuildSaveRows_ActiveEffectWithoutTimer(t *testing.T) {
	rows := BuildSaveRows(
		[]ActiveEffect{{Skill: ref(1001, 1), ReuseGroup: 1, Count: 3, Time: 20}},
		nil, 0,
	)

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	want := SaveRow{Skill: ref(1001, 1), EffectCount: 3, EffectCurTime: 20, RestoreType: RestoreTypeEffect, ClassIndex: 0, BuffIndex: 1}
	if rows[0] != want {
		t.Errorf("rows[0] = %+v, want %+v", rows[0], want)
	}
}

func TestBuildSaveRows_ActiveEffectCarriesItsReuseTimer(t *testing.T) {
	rows := BuildSaveRows(
		[]ActiveEffect{{Skill: ref(1001, 1), ReuseGroup: 1, Count: 3, Time: 20}},
		[]ReuseTimer{{Skill: ref(1001, 1), ReuseGroup: 1, Delay: 5000, ExpiresAt: 999999}},
		2,
	)

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].ReuseDelay != 5000 || rows[0].SystemTime != 999999 {
		t.Errorf("rows[0] reuse = (%d, %d), want (5000, 999999)", rows[0].ReuseDelay, rows[0].SystemTime)
	}
	if rows[0].ClassIndex != 2 {
		t.Errorf("rows[0].ClassIndex = %d, want 2", rows[0].ClassIndex)
	}
}

func TestBuildSaveRows_ExcludedEffectKinds(t *testing.T) {
	cases := []struct {
		name string
		eff  ActiveEffect
	}{
		{"toggle", ActiveEffect{Skill: ref(1, 1), ReuseGroup: 1, Toggle: true}},
		{"herb", ActiveEffect{Skill: ref(2, 1), ReuseGroup: 2, Herb: true}},
		{"continuous", ActiveEffect{Skill: ref(3, 1), ReuseGroup: 3, Continuous: true}},
		{"healOverTime", ActiveEffect{Skill: ref(4, 1), ReuseGroup: 4, HealOverTime: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := BuildSaveRows([]ActiveEffect{tc.eff}, nil, 0)
			if len(rows) != 0 {
				t.Errorf("len(rows) = %d, want 0 for excluded kind %s", len(rows), tc.name)
			}
		})
	}
}

func TestBuildSaveRows_ExcludedEffectClaimsReuseGroupExceptHealOverTime(t *testing.T) {
	cases := []struct {
		name string
		eff  ActiveEffect
		want int
	}{
		{"toggle", ActiveEffect{Skill: ref(1001, 1), ReuseGroup: 1, Toggle: true}, 0},
		{"herb", ActiveEffect{Skill: ref(1001, 1), ReuseGroup: 1, Herb: true}, 0},
		{"continuous", ActiveEffect{Skill: ref(1001, 1), ReuseGroup: 1, Continuous: true}, 0},
		{"healOverTime", ActiveEffect{Skill: ref(1001, 1), ReuseGroup: 1, HealOverTime: true}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := BuildSaveRows(
				[]ActiveEffect{tc.eff},
				[]ReuseTimer{{Skill: ref(1001, 1), ReuseGroup: 1, Delay: 5000, ExpiresAt: 999999}},
				0,
			)
			if len(rows) != tc.want {
				t.Fatalf("len(rows) = %d, want %d", len(rows), tc.want)
			}
		})
	}
}

func TestBuildSaveRows_DedupBySharedReuseGroup(t *testing.T) {
	rows := BuildSaveRows(
		[]ActiveEffect{
			{Skill: ref(1001, 1), ReuseGroup: 1, Count: 1, Time: 10},
			{Skill: ref(1002, 1), ReuseGroup: 1, Count: 2, Time: 20},
		},
		nil, 0,
	)

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Skill != ref(1001, 1) {
		t.Errorf("rows[0].Skill = %+v, want the first encountered effect's skill", rows[0].Skill)
	}
}

func TestBuildSaveRows_BuffIndexOrdersEffectsBeforeReuseOnly(t *testing.T) {
	rows := BuildSaveRows(
		[]ActiveEffect{{Skill: ref(1001, 1), ReuseGroup: 1, Count: 1, Time: 10}},
		[]ReuseTimer{{Skill: ref(2002, 1), ReuseGroup: 2, Delay: 1000, ExpiresAt: 5000}},
		0,
	)

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].BuffIndex != 1 || rows[0].RestoreType != RestoreTypeEffect {
		t.Errorf("rows[0] = %+v, want the effect row at BuffIndex 1", rows[0])
	}
	if rows[1].BuffIndex != 2 || rows[1].RestoreType != RestoreTypeReuseOnly {
		t.Errorf("rows[1] = %+v, want the reuse-only row at BuffIndex 2", rows[1])
	}
}

func TestBuildSaveRows_TimerAlreadyClaimedIsNotDuplicated(t *testing.T) {
	rows := BuildSaveRows(
		[]ActiveEffect{{Skill: ref(1001, 1), ReuseGroup: 1, Count: 1, Time: 10}},
		[]ReuseTimer{{Skill: ref(1001, 1), ReuseGroup: 1, Delay: 1000, ExpiresAt: 5000}},
		0,
	)

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1 (the effect row absorbs the reuse timer, no separate reuse-only row)", len(rows))
	}
}

func TestBuildRestorePlan_UnknownSkillIsSkippedEntirely(t *testing.T) {
	rows := []SaveRow{{Skill: ref(9999, 1), SystemTime: 1_000_000, RestoreType: RestoreTypeEffect}}
	plan := BuildRestorePlan(rows, 0, func(modelskill.Ref) (bool, bool) { return false, false })

	if len(plan.Reuse) != 0 || len(plan.Effects) != 0 {
		t.Errorf("plan = %+v, want empty for an unresolved skill", plan)
	}
}

func TestBuildRestorePlan_EffectRowWithRemainingReuseRestoresBoth(t *testing.T) {
	rows := []SaveRow{{
		Skill: ref(1001, 1), EffectCount: 3, EffectCurTime: 20,
		ReuseDelay: 5000, SystemTime: 100_100, RestoreType: RestoreTypeEffect,
	}}
	plan := BuildRestorePlan(rows, 100_000, func(modelskill.Ref) (bool, bool) { return true, true })

	if len(plan.Reuse) != 1 || plan.Reuse[0] != (ReusePlan{Skill: ref(1001, 1), Delay: 5000, ExpiresAt: 100_100}) {
		t.Errorf("plan.Reuse = %+v, want one reinstated reuse timer", plan.Reuse)
	}
	if len(plan.Effects) != 1 || plan.Effects[0] != (EffectPlan{Skill: ref(1001, 1), Count: 3, Time: 20}) {
		t.Errorf("plan.Effects = %+v, want one reapplied effect", plan.Effects)
	}
}

func TestBuildRestorePlan_ReuseWithin10msIsNotRestored(t *testing.T) {
	rows := []SaveRow{{Skill: ref(1001, 1), SystemTime: 100_005, RestoreType: RestoreTypeEffect}}
	plan := BuildRestorePlan(rows, 100_000, func(modelskill.Ref) (bool, bool) { return true, true })

	if len(plan.Reuse) != 0 {
		t.Errorf("plan.Reuse = %+v, want none when only 5ms remain", plan.Reuse)
	}
	// The effect itself still restores independent of the reuse delay.
	if len(plan.Effects) != 1 {
		t.Errorf("plan.Effects = %+v, want the effect to still restore", plan.Effects)
	}
}

func TestBuildRestorePlan_ReuseOnlyRowNeverRestoresAnEffect(t *testing.T) {
	rows := []SaveRow{{
		Skill: ref(1001, 1), EffectCount: -1, EffectCurTime: -1,
		SystemTime: 200_000, RestoreType: RestoreTypeReuseOnly,
	}}
	plan := BuildRestorePlan(rows, 100_000, func(modelskill.Ref) (bool, bool) { return true, true })

	if len(plan.Reuse) != 1 {
		t.Errorf("plan.Reuse = %+v, want the reuse timer restored", plan.Reuse)
	}
	if len(plan.Effects) != 0 {
		t.Errorf("plan.Effects = %+v, want none for a reuse-only row", plan.Effects)
	}
}

func TestBuildRestorePlan_SkillWithoutEffectsSkipsEffectRestore(t *testing.T) {
	rows := []SaveRow{{
		Skill: ref(1001, 1), EffectCount: 3, EffectCurTime: 20,
		SystemTime: 200_000, RestoreType: RestoreTypeEffect,
	}}
	plan := BuildRestorePlan(rows, 100_000, func(modelskill.Ref) (bool, bool) { return true, false })

	if len(plan.Effects) != 0 {
		t.Errorf("plan.Effects = %+v, want none when the skill carries no effect templates", plan.Effects)
	}
}

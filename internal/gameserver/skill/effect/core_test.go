package effect

import (
	"reflect"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

type funcOwner struct {
	funcs []basefunc.Func
}

func (o *funcOwner) AddStatFuncs(funcs []basefunc.Func) {
	o.funcs = append(o.funcs, funcs...)
}

func (o *funcOwner) RemoveStatsByOwner(any) {}

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
	if e.Funcs[0].Owner() != e {
		t.Fatal("compiled func owner is not the runtime effect")
	}
	if e.Funcs[0].Stat() != stat.RunSpeed {
		t.Fatalf("first func stat = %s, want runSpd", e.Funcs[0].Stat())
	}
	if got := e.Funcs[0].Calc(nil, nil, nil, 100, 100); got != 133 {
		t.Fatalf("first func Calc() = %v, want 133", got)
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
		{"Debuff", TypeDebuff, FlagNone, true, false},
		{"Stun", TypeStun, FlagStunned, true, true},
		{"Root", TypeRoot, FlagRooted, true, true},
		{"Sleep", TypeSleep, FlagSleep, true, true},
		{"Fear", TypeFear, FlagFear, true, true},
		{"DamOverTime", TypeDamOverTime, FlagNone, true, false},
		{"ManaDamOverTime", TypeManaDamOverTime, FlagNone, false, false},
		{"AbortCast", TypeAbortCast, FlagNone, false, false},
		{"ImmobileUntilAttacked", TypeImmobileUntilAttacked, FlagMeditating, false, false},
		{"ImobileBuff", TypeImmobilizeEffector, FlagNone, false, false},
		{"Invincible", TypeInvincible, FlagNone, false, false},
		{"ManaHealOverTime", TypeManaHealOverTime, FlagNone, false, false},
		{"Mute", TypeMute, flagMuted, true, false},
		{"NoblesseBless", TypeNoblesseBless, flagNoblesseBlessing, false, false},
		{"Paralyze", TypeParalyze, FlagParalyzed, true, false},
		{"Petrification", TypePetrification, FlagParalyzed, true, false},
		{"PhysicalMute", TypePhysicalMute, flagPhysicalMuted, true, false},
		{"RemoveTarget", TypeRemoveTarget, FlagNone, false, false},
		{"SilenceMagicPhysical", TypeSilenceAll, flagMuted | flagPhysicalMuted, true, false},
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
		{"Betray", TypeBetray, FlagBetrayed, true, false},
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

func TestBigHeadEffectCarriesFlagWithNoHooks(t *testing.T) {
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "BigHead"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if e.OnStart != nil || e.OnExit != nil {
		t.Fatal("BigHead must carry no start/exit hooks: the abnormal-effect broadcast isn't wired anywhere yet")
	}
	if e.Flag == FlagNone {
		t.Fatal("BigHead must carry a distinct, non-zero flag")
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

func TestListRescheduleSeedsRestartsOnlyActiveSeedEffects(t *testing.T) {
	list := NewList(nil)
	target := &liveEffectTarget{list: list}

	seed, err := New(Skill{Level: 1}, modelskill.EffectTemplate{Name: "Seed", Time: 5})
	if err != nil {
		t.Fatalf("New(Seed) error: %v", err)
	}
	seed.Effected = target
	list.Add(seed)

	buff, err := New(Skill{Level: 1}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("New(Buff) error: %v", err)
	}
	buff.Effected = target
	list.Add(buff)

	seed.scheduleMu.Lock()
	seed.remaining = 0
	seed.scheduleMu.Unlock()
	buff.scheduleMu.Lock()
	buff.remaining = 0
	buff.scheduleMu.Unlock()

	list.RescheduleSeeds()

	if got := seed.Remaining(); got != seed.Template.Count {
		t.Fatalf("seed Remaining() after RescheduleSeeds = %d, want reset to %d", got, seed.Template.Count)
	}
	if got := buff.Remaining(); got != 0 {
		t.Fatalf("buff Remaining() after RescheduleSeeds = %d, want left untouched at 0", got)
	}
}

package effect

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from core_test.go ----
type funcOwner struct {
	funcs []Mod
}

func (o *funcOwner) AttachStatFuncs(funcs []Mod) {
	o.funcs = append(o.funcs, funcs...)
}

func (o *funcOwner) StatFuncsAttached([]Mod) {}

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
	newTestList(owner).Add(e)
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
		{"Mute", TypeMute, FlagMuted, false, false},
		{"NoblesseBless", TypeNoblesseBless, flagNoblesseBlessing, false, false},
		{"Paralyze", TypeParalyze, FlagParalyzed, false, false},
		{"Petrification", TypePetrification, FlagParalyzed, false, false},
		{"PhysicalMute", TypePhysicalMute, FlagPhysicalMuted, false, false},
		{"RemoveTarget", TypeRemoveTarget, FlagNone, false, false},
		{"SilenceMagicPhysical", TypeSilenceAll, FlagMuted | FlagPhysicalMuted, false, false},
		{"SilentMove", TypeSilentMove, FlagSilentMove, false, false},
		{"StunSelf", TypeStunSelf, FlagStunned, false, false},
		{"Heal", TypeHeal, FlagNone, false, false},
		{"HealOverTime", TypeHealOverTime, FlagNone, false, false},
		{"ManaHeal", TypeManaHeal, FlagNone, false, false},
		{"TargetMe", TypeTargetMe, FlagNone, false, false},
		{"Bluff", TypeBluff, FlagNone, false, false},
		{"CharmOfCourage", TypeCharmOfCourage, FlagCharmOfCourage, false, false},
		{"CharmOfLuck", TypeCharmOfLuck, FlagCharmOfLuck, false, false},
		{"PhoenixBless", TypePhoenixBless, FlagPhoenixBlessing, false, false},
		{"BlockBuff", TypeBlockBuff, FlagNone, false, false},
		{"BlockDebuff", TypeBlockDebuff, FlagNone, false, false},
		{"ProtectionBlessing", TypeProtectionBless, FlagProtectionBlessing, false, false},
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
			e.Effected = &deadTarget{}
			// Fear keeps its full count whatever each flee does.
			if got, want := e.ActionTime(), tt.wantType == TypeFear; got != want {
				t.Fatalf("ActionTime() = %v, want %v", got, want)
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

// TestBigHeadEffectTogglesMaskAndRefreshesAppearanceOnce proves BigHead's
// start/exit hooks flip the 0x002000 mask and re-announce the target's
// appearance exactly once each (Creature.startAbnormalEffect() ->
// updateAbnormalEffect()), with no separate broadcast on top.
func TestBigHeadEffectTogglesMaskAndRefreshesAppearanceOnce(t *testing.T) {
	target := &abnormalPlayerTarget{}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "BigHead"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("OnStart() = false, want true")
	}
	e.OnExit(e)

	want := []string{"start:0x2000", "abnormal", "stop:0x2000", "abnormal"}
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
	target := &abnormalPlayerTarget{}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "ClanGate"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("ClanGate OnStart() = false, want true")
	}
	want := []string{fmt.Sprintf("start:%#x", magicCircleAbnormalMask), "abnormal"}
	if !reflect.DeepEqual(target.events, want) {
		t.Fatalf("start events = %#v, want %#v", target.events, want)
	}

	e.OnExit(e)
	want = append(want, fmt.Sprintf("stop:%#x", magicCircleAbnormalMask), "abnormal")
	if !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events after exit = %#v, want %#v", target.events, want)
	}
}

type growEffectTarget struct {
	world.Presence
	neutralActor
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

func (*growEffectTarget) Kind() actor.Kind { return actor.KindNPC }

func (funcOwner) NotifyEffectAborted(modelskill.ID, int) {}

func (funcOwner) NotifyEffectFelt(modelskill.ID, int) {}

func (funcOwner) NotifyEffectDisappeared(modelskill.ID, int) {}

func (funcOwner) NotifyEffectWornOff(modelskill.ID, int) {}

func (funcOwner) UpdateEffectIcons() {}

// playerStubs supplies the player-only effect surface as no-ops, so a fake
// that records abnormal-mask changes and appearance refreshes can stand in
// for a player target.
type playerStubs struct{}

func (playerStubs) IncreaseCharges(int, int) bool        { return false }
func (playerStubs) CurrentTarget() world.Tracked         { return nil }
func (playerStubs) SetTarget(world.Tracked)              {}
func (playerStubs) TryToAttack(world.Tracked)            {}
func (playerStubs) WakeAI()                              {}
func (playerStubs) StopCharmOfLuck(*Effect)              {}
func (playerStubs) StopPhoenixBlessing(*Effect)          {}
func (playerStubs) StopProtectionBlessing(*Effect)       {}
func (playerStubs) BroadcastEtcStatus()                  {}
func (playerStubs) WeaponGradePenalty() bool             { return false }
func (playerStubs) ReduceDeathPenaltyLevel() int         { return 0 }
func (playerStubs) CastingNow() bool                     { return false }
func (playerStubs) CurrentSkillIsMagic() bool            { return false }
func (playerStubs) InterruptCast()                       {}
func (playerStubs) StopCast()                            {}
func (playerStubs) Standing() bool                       { return false }
func (playerStubs) SetStanding(bool) bool                { return false }
func (playerStubs) Sit() bool                            { return false }
func (playerStubs) StartFakeDeath() bool                 { return false }
func (playerStubs) StopFakeDeath() bool                  { return false }
func (playerStubs) MarkRecentFakeDeath()                 {}
func (playerStubs) HPFull() bool                         { return false }
func (playerStubs) BroadcastStatus()                     {}
func (playerStubs) SendRegenMax(int32, int32, float64)   {}
func (playerStubs) NotifyEffectRemovedDueLackHP(*Effect) {}
func (playerStubs) NotifyEffectRemovedDueLackMP(*Effect) {}
func (playerStubs) NotifyRelaxDeactivatedHPFull(*Effect) {}
func (playerStubs) NotifyHPRestored(string, int, bool)   {}
func (playerStubs) NotifyMPRestored(string, int, bool)   {}
func (playerStubs) NotifySpoilAlready()                  {}
func (playerStubs) NotifySpoilSuccess()                  {}

func (playerStubs) ReduceHPByToggleUpkeep(float64, Actor) {}

// abnormalPlayerTarget is a player-shaped growEffectTarget.
type abnormalPlayerTarget struct {
	growEffectTarget
	playerStubs
}

var _ PlayerActor = (*abnormalPlayerTarget)(nil)

// deadTarget is a neutral, dead effect target: every periodic hook acting on
// it stops.
type deadTarget struct {
	neutralActor
	world.Presence
}

func (*deadTarget) Dead() bool { return true }

// TestPeriodRemainingTracksCurrentTickPeriod pins the remaining-duration
// input the debuff-cancel roll reads: the template period minus the whole
// seconds elapsed since the current period started (EffectCancelDebuff
// calcCancelSuccess: getPeriod() - getTime(), getTime() truncated to whole
// seconds and reset on every tick).
func TestPeriodRemainingTracksCurrentTickPeriod(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	e := &Effect{Template: modelskill.EffectTemplate{Name: "Debuff", Time: 3600, Count: 2}}
	e.startSchedule(start)

	for _, tc := range []struct {
		name string
		at   time.Duration
		want int
	}{
		{name: "fresh", at: 0, want: 3600},
		{name: "partial second truncates elapsed", at: 1500*time.Second + 900*time.Millisecond, want: 2100},
		{name: "last whole second", at: 3599 * time.Second, want: 1},
	} {
		if got := e.periodRemaining(start.Add(tc.at)); got != tc.want {
			t.Errorf("%s: periodRemaining = %d, want %d", tc.name, got, tc.want)
		}
	}

	// A tick restarts the period, as the reference resets its period start
	// on every scheduled action.
	if run, _ := e.claimAction(start.Add(3600 * time.Second)); !run {
		t.Fatal("first tick did not run")
	}
	if got := e.periodRemaining(start.Add(3700 * time.Second)); got != 3500 {
		t.Errorf("after first tick: periodRemaining = %d, want 3500", got)
	}

	// The remaining time drops below the 1200-second step the debuff-cancel
	// rate adds per whole unit, so the same candidate rolls lower late in its
	// period than a full-duration input would give it.
	late := e.periodRemaining(start.Add(6100 * time.Second))
	if got, full := formulas.EffectCancelDebuffSuccessRate(40, 20, late, 1), formulas.EffectCancelDebuffSuccessRate(40, 20, e.Template.Time, 1); got != 40 || full != 43 {
		t.Errorf("late rate = %d (remaining %d), full-duration rate = %d, want 40 and 43", got, late, full)
	}

	unscheduled := &Effect{Template: modelskill.EffectTemplate{Name: "Debuff", Time: 0}}
	unscheduled.startSchedule(start)
	if got := unscheduled.periodRemaining(start.Add(time.Hour)); got != 0 {
		t.Errorf("no-period effect: periodRemaining = %d, want 0", got)
	}
}

// maskRecordingTarget is a liveEffectTarget that also records the abnormal
// mask set and cleared on it, so the template-abnormal chain can be ordered
// against a kind's own hook events.
type maskRecordingTarget struct {
	*liveEffectTarget
}

func (t maskRecordingTarget) StartAbnormalEffect(mask int) {
	t.events = append(t.events, fmt.Sprintf("start:%#x", mask))
}

func (t maskRecordingTarget) StopAbnormalEffect(mask int) {
	t.events = append(t.events, fmt.Sprintf("stop:%#x", mask))
}

// TestTemplateAbnormalChainsAfterKindHooks pins wireTemplateAbnormal's
// ownership split: a kind whose hook defers to the default one runs its own
// work first and then sets (start) or clears (exit) the template mask; a
// kind that owns a hook outright never touches the mask there, while its
// other, default hook still does.
func TestTemplateAbnormalChainsAfterKindHooks(t *testing.T) {
	const mask = 0x4000
	set, clear := fmt.Sprintf("start:%#x", mask), fmt.Sprintf("stop:%#x", mask)
	tests := []struct {
		name      string
		setup     func(*liveEffectTarget)
		wantStart []string
		wantExit  []string
	}{
		// Own start and exit work, then the default hook's mask.
		{name: "Invincible", wantStart: []string{"invul:true", set, "abnormal"}, wantExit: []string{"invul:false", clear, "abnormal"}},
		// No hooks of its own: the mask is all it does.
		{name: "Buff", wantStart: []string{set, "abnormal"}, wantExit: []string{clear, "abnormal"}},
		// Owns both hooks: the mask never appears.
		{name: "Stun", wantStart: []string{"abort:false", "idle", "abnormal"}, wantExit: []string{"abnormal"}},
		// Owns only its start hook: no set on start, the default exit still clears.
		{name: "Heal", setup: func(t *liveEffectTarget) { t.canBeHealed, t.healEffectiveness = true, 100 }, wantStart: []string{"add-hp:10", "add-hp:10"}, wantExit: []string{clear, "abnormal"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := &liveEffectTarget{list: newTestList(nil)}
			if tt.setup != nil {
				tt.setup(inner)
			}
			e, err := New(Skill{}, modelskill.EffectTemplate{Name: tt.name, Value: 10, AbnormalEffect: mask})
			if err != nil {
				t.Fatalf("New(%q) error: %v", tt.name, err)
			}
			e.Effected = maskRecordingTarget{inner}

			if !e.OnStart(e) {
				t.Fatalf("%s OnStart() = false, want true", tt.name)
			}
			if !reflect.DeepEqual(inner.events, tt.wantStart) {
				t.Fatalf("%s start events = %#v, want %#v", tt.name, inner.events, tt.wantStart)
			}
			inner.events = nil
			e.OnExit(e)
			if !reflect.DeepEqual(inner.events, tt.wantExit) {
				t.Fatalf("%s exit events = %#v, want %#v", tt.name, inner.events, tt.wantExit)
			}
		})
	}
}

// TestTemplateAbnormalSkippedWhenKindDeclinesStart proves a chained start
// hook that refuses the effect leaves the template mask unset: the default
// hook runs only after the kind's own work succeeded.
func TestTemplateAbnormalSkippedWhenKindDeclinesStart(t *testing.T) {
	inner := &liveEffectTarget{list: newTestList(nil)}
	e := &Effect{
		Type:     TypeInvincible,
		Template: modelskill.EffectTemplate{AbnormalEffect: 0x4000},
		Effected: maskRecordingTarget{inner},
		OnStart: func(e *Effect) bool {
			inner.events = append(inner.events, "declined")
			return false
		},
	}
	wireTemplateAbnormal(e)

	if e.OnStart(e) {
		t.Fatal("OnStart() = true, want false: the kind's own hook declined")
	}
	if want := []string{"declined"}; !reflect.DeepEqual(inner.events, want) {
		t.Fatalf("events = %#v, want %#v: a declined start must not set the template mask", inner.events, want)
	}
}

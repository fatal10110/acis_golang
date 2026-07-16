package effect

import (
	"fmt"
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
		name     string
		wantType Type
		wantFlag Flag
		debuff   bool
	}{
		{"Debuff", TypeDebuff, FlagNone, true},
		{"Stun", TypeStun, FlagStunned, true},
		{"Root", TypeRoot, FlagRooted, true},
		{"Sleep", TypeSleep, FlagSleep, true},
		{"Fear", TypeFear, FlagFear, true},
		{"DamOverTime", TypeDamOverTime, FlagNone, true},
		{"ManaDamOverTime", TypeManaDamOverTime, FlagNone, false},
		{"AbortCast", TypeAbortCast, FlagNone, false},
		{"ImmobileUntilAttacked", TypeImmobileUntilAttacked, flagMeditating, false},
		{"ImobileBuff", TypeImmobilizeEffector, FlagNone, false},
		{"Invincible", TypeInvincible, FlagNone, false},
		{"ManaHealOverTime", TypeManaHealOverTime, FlagNone, false},
		{"Mute", TypeMute, flagMuted, true},
		{"NoblesseBless", TypeNoblesseBless, flagNoblesseBlessing, false},
		{"Paralyze", TypeParalyze, flagParalyzed, true},
		{"Petrification", TypePetrification, flagParalyzed, true},
		{"PhysicalMute", TypePhysicalMute, flagPhysicalMuted, true},
		{"RemoveTarget", TypeRemoveTarget, FlagNone, false},
		{"SilenceMagicPhysical", TypeSilenceAll, flagMuted | flagPhysicalMuted, true},
		{"SilentMove", TypeSilentMove, flagSilentMove, false},
		{"StunSelf", TypeStunSelf, FlagStunned, false},
		{"Heal", TypeHeal, FlagNone, false},
		{"HealOverTime", TypeHealOverTime, FlagNone, false},
		{"ManaHeal", TypeManaHeal, FlagNone, false},
		{"TargetMe", TypeTargetMe, FlagNone, false},
		{"Bluff", TypeBluff, FlagNone, false},
		{"CharmOfCourage", TypeCharmOfCourage, flagCharmOfCourage, false},
		{"CharmOfLuck", TypeCharmOfLuck, flagCharmOfLuck, false},
		{"PhoenixBless", TypePhoenixBless, flagPhoenixBlessing, false},
		{"BlockBuff", TypeBlockBuff, FlagNone, false},
		{"BlockDebuff", TypeBlockDebuff, FlagNone, false},
		{"ProtectionBlessing", TypeProtectionBless, flagProtectionBlessing, false},
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
			if e.ActionTime() {
				t.Fatal("non-periodic action hook continued")
			}
		})
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

func TestDisablerEffectsRunLiveStartExitHooks(t *testing.T) {
	tests := []struct {
		name      string
		wantStart []string
		wantExit  []string
	}{
		{
			name:      "Stun",
			wantStart: []string{"abort:false", "idle", "abnormal"},
			wantExit:  []string{"abnormal"},
		},
		{
			name:      "Root",
			wantStart: []string{"stop-move", "abnormal"},
			wantExit:  []string{"think", "abnormal"},
		},
		{
			name:      "Sleep",
			wantStart: []string{"abort:false", "abnormal"},
			wantExit:  []string{"think", "abnormal"},
		},
		{
			name:      "Paralyze",
			wantStart: []string{"abort:false"},
			wantExit:  []string{"think"},
		},
		{
			name:      "Petrification",
			wantStart: []string{"abort:false", "invul:true"},
			wantExit:  []string{"think", "invul:false"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &liveEffectTarget{}
			e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: tt.name})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			e.Effected = target
			list := NewList(nil)

			list.Add(e)
			if !reflect.DeepEqual(target.events, tt.wantStart) {
				t.Fatalf("start events = %#v, want %#v", target.events, tt.wantStart)
			}

			target.events = nil
			list.Remove(e)
			if !reflect.DeepEqual(target.events, tt.wantExit) {
				t.Fatalf("exit events = %#v, want %#v", target.events, tt.wantExit)
			}
		})
	}
}

func TestFearEffectHooksFleeAndRejectImmuneTargets(t *testing.T) {
	target := &liveEffectTarget{}
	e, err := New(Skill{ID: 1092}, modelskill.EffectTemplate{Name: "Fear"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effector = "caster"
	e.Effected = target

	list := NewList(nil)
	list.Add(e)
	if want := []string{"abort:false", "abnormal", "flee:caster:500"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("fear start events = %#v, want %#v", target.events, want)
	}

	target.events = nil
	if !e.ActionTime() {
		t.Fatal("fear action hook stopped, want continuing flee ticks")
	}
	if want := []string{"flee:caster:500"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("fear action events = %#v, want %#v", target.events, want)
	}

	target.events = nil
	list.Remove(e)
	if want := []string{"stop-effects:FEAR", "abnormal"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("fear exit events = %#v, want %#v", target.events, want)
	}

	immune := &liveEffectTarget{fearImmune: true}
	blocked, err := New(Skill{ID: 1092}, modelskill.EffectTemplate{Name: "Fear"})
	if err != nil {
		t.Fatalf("New() immune error: %v", err)
	}
	blocked.Effected = immune
	blockedList := NewList(nil)
	blockedList.Add(blocked)
	if blocked.InUse() {
		t.Fatal("blocked fear effect is in use")
	}
	if got := len(blockedList.All()); got != 0 {
		t.Fatalf("blocked fear effects in list = %d, want 0", got)
	}

	playable := &liveEffectTarget{playable: true}
	skipped, err := New(Skill{ID: 98}, modelskill.EffectTemplate{Name: "Fear", StackType: "turn_flee", StackOrder: 1})
	if err != nil {
		t.Fatalf("New() playable skip error: %v", err)
	}
	skipped.Effected = playable
	skippedList := NewList(nil)
	skippedList.Add(skipped)
	if skipped.InUse() {
		t.Fatal("playable-skipped fear effect is in use")
	}
	if got := len(skippedList.All()); got != 0 {
		t.Fatalf("playable-skipped fear effects in list = %d, want 0", got)
	}
}

func TestDamageOverTimeTick(t *testing.T) {
	tests := []struct {
		name string
		in   DamageOverTimeInput
		want DamageOverTimeResult
	}{
		{
			name: "dead target stops",
			in:   DamageOverTimeInput{Dead: true, HP: 10, Damage: 3},
			want: DamageOverTimeResult{Continue: false},
		},
		{
			name: "damage below hp applies",
			in:   DamageOverTimeInput{HP: 10, Damage: 3},
			want: DamageOverTimeResult{Damage: 3, Continue: true},
		},
		{
			name: "non-lethal dot leaves one hp",
			in:   DamageOverTimeInput{HP: 10, Damage: 10},
			want: DamageOverTimeResult{Damage: 9, Continue: true},
		},
		{
			name: "non-lethal dot keeps ticking at one hp",
			in:   DamageOverTimeInput{HP: 1, Damage: 5},
			want: DamageOverTimeResult{Continue: true},
		},
		{
			name: "lethal dot can consume remaining hp",
			in:   DamageOverTimeInput{HP: 10, Damage: 10, KillByDOT: true},
			want: DamageOverTimeResult{Damage: 10, Continue: true},
		},
		{
			name: "toggle stops before consuming lethal hp",
			in:   DamageOverTimeInput{HP: 10, Damage: 10, Toggle: true},
			want: DamageOverTimeResult{Continue: false, RemovedForLackHP: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DamageOverTimeTick(tt.in); got != tt.want {
				t.Fatalf("DamageOverTimeTick() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDamageOverTimeHookMutatesLiveTarget(t *testing.T) {
	target := &liveEffectTarget{hp: 10}
	e, err := New(Skill{ID: 4082}, modelskill.EffectTemplate{Name: "DamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effector = "caster"
	e.Effected = target

	if !e.ActionTime() {
		t.Fatal("DoT action hook stopped on live target")
	}
	if target.hp != 6 {
		t.Fatalf("target hp = %v, want 6", target.hp)
	}
	if want := []string{"dot:4:caster:4082"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("DoT events = %#v, want %#v", target.events, want)
	}

	target.hp = 3
	target.events = nil
	e.Template.Value = 5
	if !e.ActionTime() {
		t.Fatal("non-lethal DoT action stopped at low hp")
	}
	if target.hp != 1 {
		t.Fatalf("low-hp target hp = %v, want 1", target.hp)
	}
	if want := []string{"dot:2:caster:4082"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("low-hp DoT events = %#v, want %#v", target.events, want)
	}

	target.hp = 1
	target.events = nil
	if !e.ActionTime() {
		t.Fatal("DoT at one hp stopped, want continuing without damage")
	}
	if len(target.events) != 0 {
		t.Fatalf("one-hp DoT events = %#v, want none", target.events)
	}

	target.hp = 10
	target.events = nil
	e.Template.Value = 10
	e.Skill.Toggle = true
	if e.ActionTime() {
		t.Fatal("toggle DoT action continued after lethal tick, want stop")
	}
	if want := []string{"lack-hp"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("toggle DoT events = %#v, want %#v", target.events, want)
	}
}

func TestManaDamageOverTimeTick(t *testing.T) {
	tests := []struct {
		name string
		in   ManaDamageOverTimeInput
		want ManaDamageOverTimeResult
	}{
		{
			name: "dead target stops",
			in:   ManaDamageOverTimeInput{Dead: true, MP: 10, Damage: 3},
			want: ManaDamageOverTimeResult{Continue: false},
		},
		{
			name: "damage below mp applies",
			in:   ManaDamageOverTimeInput{MP: 10, Damage: 3},
			want: ManaDamageOverTimeResult{Damage: 3, Continue: true},
		},
		{
			name: "non-toggle drain always pays even past mp",
			in:   ManaDamageOverTimeInput{MP: 5, Damage: 10},
			want: ManaDamageOverTimeResult{Damage: 10, Continue: true},
		},
		{
			name: "toggle upkeep exactly matching mp still pays",
			in:   ManaDamageOverTimeInput{MP: 10, Damage: 10, Toggle: true},
			want: ManaDamageOverTimeResult{Damage: 10, Continue: true},
		},
		{
			name: "toggle upkeep exceeding mp drops instead of paying",
			in:   ManaDamageOverTimeInput{MP: 9, Damage: 10, Toggle: true},
			want: ManaDamageOverTimeResult{Continue: false, RemovedForLackMP: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ManaDamageOverTimeTick(tt.in); got != tt.want {
				t.Fatalf("ManaDamageOverTimeTick() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestManaDamageOverTimeHookMutatesLiveTarget(t *testing.T) {
	target := &liveEffectTarget{mp: 20}
	e, err := New(Skill{ID: 288}, modelskill.EffectTemplate{Name: "ManaDamOverTime", Value: 8})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.ActionTime() {
		t.Fatal("mana DoT action hook stopped on live target")
	}
	if target.mp != 12 {
		t.Fatalf("target mp = %v, want 12", target.mp)
	}
	if want := []string{"mpdot:8"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("mana DoT events = %#v, want %#v", target.events, want)
	}

	target.mp = 9
	target.events = nil
	e.Skill.Toggle = true
	e.Template.Value = 10
	if e.ActionTime() {
		t.Fatal("toggle mana DoT action continued past available mp, want stop")
	}
	if want := []string{"lack-mp"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("toggle mana DoT events = %#v, want %#v", target.events, want)
	}
	if target.mp != 9 {
		t.Fatalf("target mp after lack-mp drop = %v, want unchanged 9", target.mp)
	}
}

func TestAbortCastEffectHook(t *testing.T) {
	tests := []struct {
		name        string
		selfCast    bool
		raidRelated bool
		castingNow  bool
		wantEvents  []string
		wantInUse   bool
	}{
		{
			name:       "interrupts an in-progress cast",
			castingNow: true,
			wantEvents: []string{"interrupt-cast"},
			wantInUse:  true,
		},
		{
			name:       "no-ops when target is not casting",
			castingNow: false,
			wantEvents: nil,
			wantInUse:  true,
		},
		{
			name:       "rejected on self-cast",
			selfCast:   true,
			castingNow: true,
			wantEvents: nil,
			wantInUse:  false,
		},
		{
			name:        "rejected on a raid-related target",
			raidRelated: true,
			castingNow:  true,
			wantEvents:  nil,
			wantInUse:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &liveEffectTarget{castingNow: tt.castingNow, raidRelated: tt.raidRelated}
			e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "AbortCast"})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			e.Effected = target
			if tt.selfCast {
				e.Effector = target
			} else {
				e.Effector = "caster"
			}

			NewList(nil).Add(e)
			if !reflect.DeepEqual(target.events, tt.wantEvents) {
				t.Fatalf("events = %#v, want %#v", target.events, tt.wantEvents)
			}
			if e.InUse() != tt.wantInUse {
				t.Fatalf("InUse() = %v, want %v", e.InUse(), tt.wantInUse)
			}
		})
	}
}

func TestMuteFamilyEffectsStopMatchingCastOnly(t *testing.T) {
	tests := []struct {
		name       string
		effect     string
		castingNow bool
		castMagic  bool
		wantEvents []string
	}{
		{name: "Mute interrupts a magic cast", effect: "Mute", castingNow: true, castMagic: true, wantEvents: []string{"stop-cast", "abnormal"}},
		{name: "Mute ignores a physical cast", effect: "Mute", castingNow: true, castMagic: false, wantEvents: []string{"abnormal"}},
		{name: "PhysicalMute interrupts a physical cast", effect: "PhysicalMute", castingNow: true, castMagic: false, wantEvents: []string{"stop-cast", "abnormal"}},
		{name: "PhysicalMute ignores a magic cast", effect: "PhysicalMute", castingNow: true, castMagic: true, wantEvents: []string{"abnormal"}},
		{name: "SilenceMagicPhysical stops any cast unconditionally", effect: "SilenceMagicPhysical", castingNow: true, castMagic: true, wantEvents: []string{"stop-cast", "abnormal"}},
		{name: "SilenceMagicPhysical stops even when the target reports idle", effect: "SilenceMagicPhysical", castingNow: false, wantEvents: []string{"stop-cast", "abnormal"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &liveEffectTarget{castingNow: tt.castingNow, castMagic: tt.castMagic}
			e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: tt.effect})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			e.Effected = target

			NewList(nil).Add(e)
			if !reflect.DeepEqual(target.events, tt.wantEvents) {
				t.Fatalf("events = %#v, want %#v", target.events, tt.wantEvents)
			}
		})
	}
}

func TestImmobileUntilAttackedEffectLifecycle(t *testing.T) {
	target := &liveEffectTarget{}
	e, err := New(Skill{ID: 77}, modelskill.EffectTemplate{Name: "ImmobileUntilAttacked"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	list := NewList(nil)
	list.Add(e)
	if want := []string{"abort:false", "abnormal"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("start events = %#v, want %#v", target.events, want)
	}

	target.events = nil
	if e.ActionTime() {
		t.Fatal("immobile-until-attacked action hook continued, want a one-shot end")
	}
	if want := []string{"stop-skill:77", "think", "abnormal"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("action events = %#v, want %#v", target.events, want)
	}

	target.events = nil
	list.Remove(e)
	if want := []string{"stop-skill:77", "think", "abnormal"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("exit events = %#v, want %#v", target.events, want)
	}
}

func TestImmobilizeEffectorEffectTargetsEffectorNotEffected(t *testing.T) {
	effected := &liveEffectTarget{}
	effector := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "ImobileBuff"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = effected
	e.Effector = effector

	list := NewList(nil)
	list.Add(e)
	if want := []string{"immobilized:true"}; !reflect.DeepEqual(effector.events, want) {
		t.Fatalf("effector start events = %#v, want %#v", effector.events, want)
	}
	if len(effected.events) != 0 {
		t.Fatalf("effected events = %#v, want none", effected.events)
	}

	list.Remove(e)
	if want := []string{"immobilized:true", "immobilized:false"}; !reflect.DeepEqual(effector.events, want) {
		t.Fatalf("effector exit events = %#v, want %#v", effector.events, want)
	}
}

func TestInvincibleEffectTogglesInvulOnStartAndExit(t *testing.T) {
	target := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "Invincible"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	list := NewList(nil)
	list.Add(e)
	if want := []string{"invul:true"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("start events = %#v, want %#v", target.events, want)
	}

	list.Remove(e)
	if want := []string{"invul:true", "invul:false"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("exit events = %#v, want %#v", target.events, want)
	}
}

func TestRemoveTargetEffectClearsTargetAttackAndCast(t *testing.T) {
	target := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "RemoveTarget"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	NewList(nil).Add(e)
	want := []string{"clear-target", "stop-attack", "stop-cast"}
	if !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events = %#v, want %#v", target.events, want)
	}
}

func TestSilentMoveActionOnlyTicksContSkillsAndStopsOnLowMana(t *testing.T) {
	target := &liveEffectTarget{mp: 10}
	e, err := New(Skill{ID: 1, SkillType: "CONT"}, modelskill.EffectTemplate{Name: "SilentMove", Value: 4})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.ActionTime() {
		t.Fatal("silent move action stopped on a CONT skill with enough mana")
	}
	if target.mp != 6 {
		t.Fatalf("target mp = %v, want 6", target.mp)
	}
	if want := []string{"mpdot:4"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events = %#v, want %#v", target.events, want)
	}

	target.events = nil
	target.mp = 2
	if e.ActionTime() {
		t.Fatal("silent move action continued with insufficient mana, want stop")
	}
	if want := []string{"lack-mp"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("low-mana events = %#v, want %#v", target.events, want)
	}

	nonCont, err := New(Skill{ID: 1, SkillType: "BUFF"}, modelskill.EffectTemplate{Name: "SilentMove", Value: 4})
	if err != nil {
		t.Fatalf("New() non-CONT error: %v", err)
	}
	nonCont.Effected = &liveEffectTarget{mp: 10}
	if nonCont.ActionTime() {
		t.Fatal("silent move action ticked on a non-CONT skill, want immediate stop")
	}
}

func TestManaHealOverTimeEffectHookMutatesLiveTarget(t *testing.T) {
	heal := &liveEffectTarget{mp: 1, canBeHealed: true}
	hot, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "ManaHealOverTime", Value: 5})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	hot.Effected = heal
	if !hot.ActionTime() {
		t.Fatal("mana heal action stopped on a healable target")
	}
	if heal.mp != 6 {
		t.Fatalf("target mp = %v, want 6", heal.mp)
	}
	if want := []string{"add-mp:5"}; !reflect.DeepEqual(heal.events, want) {
		t.Fatalf("events = %#v, want %#v", heal.events, want)
	}

	heal.canBeHealed = false
	heal.events = nil
	if hot.ActionTime() {
		t.Fatal("mana heal action continued on an unhealable target")
	}
	if len(heal.events) != 0 {
		t.Fatalf("events = %#v, want none", heal.events)
	}
}

func TestStunSelfEffectIdlesEffectedAndRefreshesEffector(t *testing.T) {
	effected := &liveEffectTarget{playable: true}
	effector := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "StunSelf"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = effected
	e.Effector = effector

	list := NewList(nil)
	list.Add(e)
	if want := []string{"idle"}; !reflect.DeepEqual(effected.events, want) {
		t.Fatalf("effected start events = %#v, want %#v", effected.events, want)
	}
	if want := []string{"abnormal"}; !reflect.DeepEqual(effector.events, want) {
		t.Fatalf("effector start events = %#v, want %#v", effector.events, want)
	}

	list.Remove(e)
	if want := []string{"abnormal", "abnormal"}; !reflect.DeepEqual(effector.events, want) {
		t.Fatalf("effector exit events = %#v, want %#v", effector.events, want)
	}
}

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
	target            any
	heading           int
	bluffExempt       bool
	isPlayer          bool
	list              *List
	vuln              float64
}

func (t *liveEffectTarget) EffectList() *List { return t.list }

func (t *liveEffectTarget) CancelVulnerability(classification string) float64 { return t.vuln }

func (t *liveEffectTarget) Dead() bool { return t.dead }

func (t *liveEffectTarget) HP() float64 { return t.hp }

func (t *liveEffectTarget) MP() float64 { return t.mp }

func (t *liveEffectTarget) ReduceHPByDOT(damage float64, effector any, skill Skill) {
	t.hp -= damage
	t.events = append(t.events, fmt.Sprintf("dot:%g:%v:%d", damage, effector, skill.ID))
}

func (t *liveEffectTarget) ReduceMP(damage float64) {
	t.mp -= damage
	t.events = append(t.events, fmt.Sprintf("mpdot:%g", damage))
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

func (t *liveEffectTarget) Think() {
	t.events = append(t.events, "think")
}

func (t *liveEffectTarget) Afraid() bool { return t.afraid }

func (t *liveEffectTarget) FearImmune() bool { return t.fearImmune }

func (t *liveEffectTarget) Playable() bool { return t.playable }

func (t *liveEffectTarget) FleeFrom(effector any, distance int) {
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

func (t *liveEffectTarget) SetImmobilized(v bool) {
	t.events = append(t.events, fmt.Sprintf("immobilized:%v", v))
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

func (t *liveEffectTarget) CurrentTarget() any { return t.target }

func (t *liveEffectTarget) SetTarget(target any) {
	t.target = target
	t.events = append(t.events, fmt.Sprintf("set-target:%v", target))
}

func (t *liveEffectTarget) TryToAttack(target any) {
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

// noBonusHealTarget implements only the minimum heal capability, to
// exercise the healStart/manaHealStart fallback defaults when the optional
// proficiency/effectiveness/recharge hooks are absent.
type noBonusHealTarget struct {
	hp          float64
	mp          float64
	canBeHealed bool
}

func (t *noBonusHealTarget) CanBeHealed() bool { return t.canBeHealed }

func (t *noBonusHealTarget) AddHP(amount float64) float64 {
	t.hp += amount
	return amount
}

func (t *noBonusHealTarget) AddMP(amount float64) float64 {
	t.mp += amount
	return amount
}

func TestHealEffectAppliesProficiencyAndEffectivenessAndDoublesAmount(t *testing.T) {
	target := &liveEffectTarget{canBeHealed: true, healProficiency: 10, healEffectiveness: 50}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "Heal", Value: 100})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("heal effect start rejected a healable target")
	}
	// power = 100 + 10 = 110; first add = 110 * 50/100 = 55; then the
	// amount (55) is applied a second time.
	if want := 55.0 + 55.0; target.hp != want {
		t.Fatalf("target hp = %v, want %v", target.hp, want)
	}
	if want := []string{"add-hp:55", "add-hp:55"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events = %#v, want %#v", target.events, want)
	}
}

func TestHealEffectDefaultsProficiencyAndEffectivenessWhenAbsent(t *testing.T) {
	target := &noBonusHealTarget{canBeHealed: true}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "Heal", Value: 40})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("heal effect start rejected a healable target")
	}
	// power = 40 + 0; first add = 40 * 100/100 = 40; doubled to 80.
	if target.hp != 80 {
		t.Fatalf("target hp = %v, want 80", target.hp)
	}
}

func TestHealEffectRejectsUnhealableTarget(t *testing.T) {
	target := &liveEffectTarget{canBeHealed: false}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "Heal", Value: 40})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if e.OnStart(e) {
		t.Fatal("heal effect started on an unhealable target")
	}
	if len(target.events) != 0 {
		t.Fatalf("events = %#v, want none", target.events)
	}
}

func TestHealOverTimeActionRestoresHPEachTick(t *testing.T) {
	target := &liveEffectTarget{canBeHealed: true, hp: 1}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "HealOverTime", Value: 7})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.ActionTime() {
		t.Fatal("heal over time action stopped on a healable target")
	}
	if target.hp != 8 {
		t.Fatalf("target hp = %v, want 8", target.hp)
	}

	target.canBeHealed = false
	target.events = nil
	if e.ActionTime() {
		t.Fatal("heal over time action continued on an unhealable target")
	}
	if len(target.events) != 0 {
		t.Fatalf("events = %#v, want none", target.events)
	}
}

func TestManaHealEffectAppliesRechargeRateAndDoublesAmount(t *testing.T) {
	target := &liveEffectTarget{canBeHealed: true, rechargeRate: func(base float64) float64 { return base * 2 }}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "ManaHeal", Value: 20})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("mana heal effect start rejected a healable target")
	}
	// power = 20 * 2 = 40, applied twice.
	if target.mp != 80 {
		t.Fatalf("target mp = %v, want 80", target.mp)
	}
	if want := []string{"add-mp:40", "add-mp:40"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events = %#v, want %#v", target.events, want)
	}
}

func TestManaHealEffectDefaultsRechargeRateWhenAbsent(t *testing.T) {
	target := &noBonusHealTarget{canBeHealed: true}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "ManaHeal", Value: 15})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("mana heal effect start rejected a healable target")
	}
	if target.mp != 30 {
		t.Fatalf("target mp = %v, want 30", target.mp)
	}
}

func TestTargetMeEffectSetsTargetOrAttacksIfAlreadyTargeted(t *testing.T) {
	effector := &liveEffectTarget{}
	target := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "TargetMe"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	e.Effector = effector

	if !e.OnStart(e) {
		t.Fatal("target me effect start rejected a valid target")
	}
	if want := []string{fmt.Sprintf("set-target:%v", effector)}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events = %#v, want %#v", target.events, want)
	}

	target.events = nil
	target.target = effector
	if !e.OnStart(e) {
		t.Fatal("target me effect start rejected a valid target")
	}
	if want := []string{fmt.Sprintf("try-attack:%v", effector)}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events = %#v, want %#v", target.events, want)
	}
}

func TestBluffEffectRedirectsHeadingUnlessExemptOrRaidRelated(t *testing.T) {
	effector := &liveEffectTarget{heading: 42}
	target := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "Bluff"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	e.Effector = effector

	if !e.OnStart(e) {
		t.Fatal("bluff effect start rejected a valid target")
	}
	if target.heading != 42 {
		t.Fatalf("target heading = %d, want 42", target.heading)
	}

	target = &liveEffectTarget{bluffExempt: true}
	e.Effected = target
	if e.OnStart(e) {
		t.Fatal("bluff effect started on an exempt target")
	}

	target = &liveEffectTarget{raidRelated: true}
	e.Effected = target
	if e.OnStart(e) {
		t.Fatal("bluff effect started on a raid-related target")
	}
}

func TestCharmOfCourageEffectOnlyStartsForPlayers(t *testing.T) {
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "CharmOfCourage"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	e.Effected = &liveEffectTarget{isPlayer: true}
	if !e.OnStart(e) {
		t.Fatal("charm of courage effect start rejected a player")
	}

	e.Effected = &liveEffectTarget{isPlayer: false}
	if e.OnStart(e) {
		t.Fatal("charm of courage effect started on a non-player")
	}
}

func TestCharmOfLuckAndPhoenixBlessNotifyOnExit(t *testing.T) {
	luck := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "CharmOfLuck"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = luck
	list := NewList(nil)
	list.Add(e)
	list.Remove(e)
	if want := []string{"stop-charm-of-luck"}; !reflect.DeepEqual(luck.events, want) {
		t.Fatalf("charm of luck exit events = %#v, want %#v", luck.events, want)
	}

	bless := &liveEffectTarget{}
	pb, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "PhoenixBless"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	pb.Effected = bless
	list2 := NewList(nil)
	list2.Add(pb)
	list2.Remove(pb)
	if want := []string{"stop-phoenix-bless"}; !reflect.DeepEqual(bless.events, want) {
		t.Fatalf("phoenix bless exit events = %#v, want %#v", bless.events, want)
	}
}

func hasEffectInList(list *List, e *Effect) bool {
	for _, cur := range list.All() {
		if cur == e {
			return true
		}
	}
	return false
}

func TestCancelEffectSkipsToggleAndDebuffCandidates(t *testing.T) {
	target := &liveEffectTarget{vuln: 1, list: NewList(nil)}
	toggle, err := New(Skill{Toggle: true}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(toggle)
	debuff, err := New(Skill{Debuff: true}, modelskill.EffectTemplate{Name: "Debuff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(debuff)

	e, err := New(Skill{MagicLevel: 80, MaxNegatedEffects: 10}, modelskill.EffectTemplate{Name: "Cancel", EffectPower: 100})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	e.OnStart(e)

	if !hasEffectInList(target.list, toggle) {
		t.Error("a toggle effect must never be stripped by a cancel effect")
	}
	if !hasEffectInList(target.list, debuff) {
		t.Error("a debuff effect must never be stripped by a cancel effect")
	}
}

func TestCancelEffectRejectsDeadTarget(t *testing.T) {
	target := &liveEffectTarget{dead: true, list: NewList(nil)}
	buff, err := New(Skill{}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(buff)

	e, err := New(Skill{MagicLevel: 80, MaxNegatedEffects: 10}, modelskill.EffectTemplate{Name: "Cancel", EffectPower: 100})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	if e.OnStart(e) {
		t.Fatal("cancel effect started on a dead target, want rejected")
	}
	if !hasEffectInList(target.list, buff) {
		t.Error("a rejected start must not touch the candidate list")
	}
}

// TestCancelEffectStripsProtectionMarkersDespiteExemptionList proves a
// deliberately preserved quirk: cancelStart's exemption check compares its
// own classification (always the cancel tag) against the protected-marker
// list, so the check can never match and the four protected markers stay
// cancellable through this path. A single trial can't distinguish "always
// removed" from "never checked" because the roll saturates below 100%, so
// this repeats many independent trials and requires at least one removal —
// with removal odds fixed at 75% per trial, the chance of zero removals
// across all of them is astronomically small.
func TestCancelEffectStripsProtectionMarkersDespiteExemptionList(t *testing.T) {
	const trials = 200
	removed := 0
	for i := 0; i < trials; i++ {
		target := &liveEffectTarget{vuln: 1, list: NewList(nil)}
		marker, err := New(Skill{}, modelskill.EffectTemplate{Name: "ProtectionBlessing", Time: 600})
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}
		target.list.Add(marker)

		e, err := New(Skill{MagicLevel: 1000, MaxNegatedEffects: 1}, modelskill.EffectTemplate{Name: "Cancel", EffectPower: 1000})
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}
		e.Effected = target
		e.OnStart(e)

		if !hasEffectInList(target.list, marker) {
			removed++
		}
	}
	if removed == 0 {
		t.Fatal("protection blessing marker was never stripped across repeated trials, want at least one removal")
	}
}

func TestNegateEffectStripsBySkillID(t *testing.T) {
	target := &liveEffectTarget{list: NewList(nil)}
	match, err := New(Skill{ID: 42}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(match)
	other, err := New(Skill{ID: 7}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(other)
	zero, err := New(Skill{ID: 0}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(zero)

	e, err := New(Skill{NegateIDs: []int{42, 0}, NegateLevel: -1}, modelskill.EffectTemplate{Name: "Negate"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	e.OnStart(e)

	if hasEffectInList(target.list, match) {
		t.Error("an effect owned by a negated skill id must be stripped")
	}
	if !hasEffectInList(target.list, other) {
		t.Error("an effect owned by an unrelated skill id must remain")
	}
	if !hasEffectInList(target.list, zero) {
		t.Error("a negateId of 0 is a no-op sentinel and must never strip anything")
	}
}

func TestNegateEffectStripsByTypeWithLevelGate(t *testing.T) {
	target := &liveEffectTarget{list: NewList(nil)}

	// Distinct skill ids matter here: the effect list treats same-id,
	// same-type, same-stack candidates as duplicates of each other and
	// silently rejects the later Add, which would hide these candidates
	// from the assertions below regardless of negateStart's behavior.
	withinLevel, err := New(Skill{ID: 1, SkillType: "POISON", AbnormalLevel: 2}, modelskill.EffectTemplate{Name: "Debuff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(withinLevel)

	aboveLevel, err := New(Skill{ID: 2, SkillType: "POISON", AbnormalLevel: 5}, modelskill.EffectTemplate{Name: "Debuff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(aboveLevel)

	wrongType, err := New(Skill{ID: 3, SkillType: "BLEED", AbnormalLevel: 1}, modelskill.EffectTemplate{Name: "Debuff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(wrongType)

	viaEffectType, err := New(Skill{ID: 4, SkillType: "BUFF", EffectType: "POISON", EffectAbnormalLevel: 2}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(viaEffectType)

	e, err := New(Skill{NegateTypes: []string{"POISON"}, NegateLevel: 3}, modelskill.EffectTemplate{Name: "Negate"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	e.OnStart(e)

	if hasEffectInList(target.list, withinLevel) {
		t.Error("a candidate within the negate level threshold should have been stripped")
	}
	if !hasEffectInList(target.list, aboveLevel) {
		t.Error("a candidate above the negate level threshold must remain")
	}
	if !hasEffectInList(target.list, wrongType) {
		t.Error("a candidate of an unrelated classification must remain")
	}
	if hasEffectInList(target.list, viaEffectType) {
		t.Error("a candidate matched via its own effectType tag should have been stripped")
	}
}

func TestNegateEffectTypeUnrestrictedWhenLevelIsMinusOne(t *testing.T) {
	target := &liveEffectTarget{list: NewList(nil)}
	high, err := New(Skill{SkillType: "POISON", AbnormalLevel: 99}, modelskill.EffectTemplate{Name: "Debuff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(high)

	e, err := New(Skill{NegateTypes: []string{"POISON"}, NegateLevel: -1}, modelskill.EffectTemplate{Name: "Negate"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	e.OnStart(e)

	if hasEffectInList(target.list, high) {
		t.Error("a negateLevel of -1 must strip regardless of abnormal level")
	}
}

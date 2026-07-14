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

type liveEffectTarget struct {
	events     []string
	hp         float64
	mp         float64
	dead       bool
	afraid     bool
	fearImmune bool
	playable   bool
}

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

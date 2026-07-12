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

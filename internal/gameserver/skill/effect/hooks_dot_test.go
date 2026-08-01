package effect

import (
	"reflect"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

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
	if want := []string{"dot:4:caster"}; !reflect.DeepEqual(target.events, want) {
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
	if want := []string{"dot:2:caster"}; !reflect.DeepEqual(target.events, want) {
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

package effect

import (
	"reflect"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

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

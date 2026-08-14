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
	// full models a target already at its maximum: the live character's
	// AddHP/AddMP apply nothing and report 0 in that state.
	full bool
}

type regenMaxTarget struct {
	liveEffectTarget
	updates [][3]float64
}

func (t *regenMaxTarget) SendRegenMax(count, period int32, hpRegen float64) {
	t.updates = append(t.updates, [3]float64{float64(count), float64(period), hpRegen})
}

func (t *noBonusHealTarget) ObjectID() int32 { return 0 }

func (t *noBonusHealTarget) Dead() bool { return false }

func (t *noBonusHealTarget) CanBeHealed() bool { return t.canBeHealed }

func (t *noBonusHealTarget) AddHP(amount float64) float64 {
	if t.full {
		return 0
	}
	t.hp += amount
	return amount
}

func (t *noBonusHealTarget) AddMP(amount float64) float64 {
	if t.full {
		return 0
	}
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

func TestHealOverTimeStartSendsRegenMaxOnlyForPlayersWithPositiveTiming(t *testing.T) {
	for _, tt := range []struct {
		name        string
		isPlayer    bool
		count, time int
		want        [][3]float64
	}{
		{name: "player", isPlayer: true, count: 3, time: 5, want: [][3]float64{{15, 5, 12.5}}},
		{name: "non-player", count: 3, time: 5},
		{name: "zero count", isPlayer: true, time: 5},
		{name: "zero period", isPlayer: true, count: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target := &regenMaxTarget{liveEffectTarget: liveEffectTarget{isPlayer: tt.isPlayer}}
			e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "HealOverTime", Count: tt.count, Time: tt.time, Value: 12.5})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			e.Effected = target
			if e.OnStart == nil {
				t.Fatal("HealOverTime has no start hook")
			}
			if !e.OnStart(e) {
				t.Fatal("HealOverTime start rejected")
			}
			if !reflect.DeepEqual(target.updates, tt.want) {
				t.Fatalf("regen updates = %#v, want %#v", target.updates, tt.want)
			}
		})
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

// broadcastingHealTarget records the status broadcasts an over-time tick
// asks for.
type broadcastingHealTarget struct {
	noBonusHealTarget
	broadcasts int
}

func (t *broadcastingHealTarget) BroadcastStatus() { t.broadcasts++ }

// TestHealOverTimeTickBroadcastsStatus pins the client notification a
// periodic heal owes its target: the tick runs outside any client request,
// so without it the bars keep showing the value the client was last told.
// A tick that applied nothing broadcasts nothing — the reference's HP/MP
// setters bypass themselves, and their status update, at zero applied, which
// is every tick of a regen buff on an already-full target.
func TestHealOverTimeTickBroadcastsStatus(t *testing.T) {
	for _, tt := range []struct {
		name           string
		kind           string
		full           bool
		wantBroadcasts int
	}{
		{name: "hp", kind: "HealOverTime", wantBroadcasts: 1},
		{name: "mp", kind: "ManaHealOverTime", wantBroadcasts: 1},
		{name: "hp already full", kind: "HealOverTime", full: true},
		{name: "mp already full", kind: "ManaHealOverTime", full: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target := &broadcastingHealTarget{noBonusHealTarget: noBonusHealTarget{canBeHealed: true, full: tt.full}}
			e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: tt.kind, Value: 10})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			e.Effected = target

			if !e.OnAction(e) {
				t.Fatalf("%s tick rejected a healable target", tt.kind)
			}
			if target.broadcasts != tt.wantBroadcasts {
				t.Errorf("status broadcasts = %d, want %d", target.broadcasts, tt.wantBroadcasts)
			}
		})
	}
}

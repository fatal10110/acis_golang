package summon

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type namedDamageAttacker struct{ name string }

func (a namedDamageAttacker) CharacterName() string { return a.name }

func TestReduceHPUpdatesStatusAfterDirectAndDOTDamage(t *testing.T) {
	for _, damage := range []struct {
		name  string
		apply func(*Actor)
	}{
		{"direct", func(a *Actor) { a.ReduceHP(10, nil, modelskill.Definition{}) }},
		{"dot", func(a *Actor) { a.ReduceHPByDOT(10, nil, true) }},
	} {
		t.Run(damage.name, func(t *testing.T) {
			a := NewPet(PetConfig{Stats: CombatStats{MaxHP: 100}})
			updates := 0
			a.SetStatusUpdater(func() { updates++ })

			damage.apply(a)

			if updates != 1 {
				t.Fatalf("status updates = %d, want 1", updates)
			}
		})
	}
}

func TestReduceHPNotifiesKnownDirectAttackerOnly(t *testing.T) {
	for _, tc := range []struct {
		name   string
		new    func() *Actor
		apply  func(*Actor, any)
		called bool
	}{
		{"pet direct", func() *Actor { return NewPet(PetConfig{Stats: CombatStats{MaxHP: 100}}) }, func(a *Actor, attacker any) { a.ReduceHP(12.9, attacker, modelskill.Definition{}) }, true},
		{"servitor direct", func() *Actor { return NewServitor(ServitorConfig{Stats: CombatStats{MaxHP: 100}}) }, func(a *Actor, attacker any) { a.ReduceHP(12.9, attacker, modelskill.Definition{}) }, true},
		{"dot", func() *Actor { return NewPet(PetConfig{Stats: CombatStats{MaxHP: 100}}) }, func(a *Actor, attacker any) { a.ReduceHPByDOT(12.9, attacker, true) }, false},
		{"unknown attacker", func() *Actor { return NewPet(PetConfig{Stats: CombatStats{MaxHP: 100}}) }, func(a *Actor, attacker any) { a.ReduceHP(12.9, attacker, modelskill.Definition{}) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotName string
			var gotDamage int32
			calls := 0
			a := tc.new()
			a.SetDamageNotifier(func(name string, damage int32) { calls++; gotName, gotDamage = name, damage })
			var attacker any = namedDamageAttacker{name: "Attacker"}
			if tc.name == "unknown attacker" {
				attacker = struct{}{}
			}
			tc.apply(a, attacker)
			if tc.called {
				if calls != 1 || gotName != "Attacker" || gotDamage != 12 {
					t.Fatalf("notification = (%d, %q, %d), want (1, Attacker, 12)", calls, gotName, gotDamage)
				}
			} else if calls != 0 {
				t.Fatalf("notifications = %d, want 0", calls)
			}
		})
	}
}

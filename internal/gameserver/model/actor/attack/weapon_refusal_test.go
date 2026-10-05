package attack

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// weaponPlayer is a player whose arrows and MP the test sets.
type weaponPlayer struct {
	timingPlayer
	arrows    bool
	mpConsume int
	mp        int
}

func (a *weaponPlayer) CheckAndEquipArrows() bool { return a.arrows }
func (a *weaponPlayer) WeaponMPConsume() int      { return a.mpConsume }
func (a *weaponPlayer) MP() int                   { return a.mp }

// TestRefuseAttackTellsWeaponRefusal pins PlayerAttack.canAttack's weapon
// gate: a fishing rod (1472), a bow with no arrows (112) and a bow with
// arrows but less MP than its cost (24) each refuse the attack and are told
// once per RefuseAttack, while CanAttack refuses the same way silently. A bow
// whose cost the player can pay, or a weapon of another type, is allowed and
// tells nothing.
func TestRefuseAttackTellsWeaponRefusal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		weapon    item.WeaponType
		arrows    bool
		mpConsume int
		mp        int
		want      event.AttackWeaponRefusal
	}{
		{"fishing rod", item.WeaponFishingRod, true, 0, 100, event.AttackRefusedFishingRod},
		{"bow without arrows", item.WeaponBow, false, 1, 100, event.AttackRefusedNoArrows},
		{"bow without MP", item.WeaponBow, true, 5, 4, event.AttackRefusedNotEnoughMP},
		{"bow with MP", item.WeaponBow, true, 5, 5, 0},
		{"sword", item.WeaponSword, false, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			player := &weaponPlayer{arrows: tc.arrows, mpConsume: tc.mpConsume, mp: tc.mp}
			player.attackType = tc.weapon
			rec := &event.Recorder{}
			ctrl := NewPlayer(player, rec)
			target := &timingTarget{id: 2, attackable: true}

			if got, want := ctrl.CanAttack(target), tc.want == 0; got != want {
				t.Fatalf("CanAttack() = %v, want %v", got, want)
			}
			if got := len(rec.Events()); got != 0 {
				t.Fatalf("CanAttack() emitted %d events, want a silent query", got)
			}
			if got, want := ctrl.RefuseAttack(target), tc.want != 0; got != want {
				t.Fatalf("RefuseAttack() = %v, want %v", got, want)
			}
			events := rec.Events()
			if tc.want == 0 {
				if len(events) != 0 {
					t.Fatalf("RefuseAttack() emitted %v, want nothing", events)
				}
				return
			}
			if len(events) != 1 || events[0] != (event.AttackWeaponRefused{Reason: tc.want}) {
				t.Fatalf("RefuseAttack() emitted %v, want one AttackWeaponRefused{%d}", events, tc.want)
			}
		})
	}
}

// TestRefuseAttackSilentWhenGateRefusesFirst: the weapon gate runs only once
// the creature rules allow the attack, so a target that may not be attacked
// refuses a fishing-rod attack without the fishing-pole message.
func TestRefuseAttackSilentWhenGateRefusesFirst(t *testing.T) {
	t.Parallel()
	player := &weaponPlayer{}
	player.attackType = item.WeaponFishingRod
	rec := &event.Recorder{}
	ctrl := NewPlayer(player, rec)

	if !ctrl.RefuseAttack(&timingTarget{id: 2}) {
		t.Fatal("RefuseAttack() = false for a target that may not be attacked")
	}
	if events := rec.Events(); len(events) != 0 {
		t.Fatalf("RefuseAttack() emitted %v, want nothing", events)
	}
}

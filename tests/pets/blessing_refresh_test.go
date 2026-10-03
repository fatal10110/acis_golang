package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// Reference: EffectCharmOfLuck.onExit / EffectPhoenixBless.onExit ->
// Playable.stopCharmOfLuck / stopPhoenixBlessing (Playable.java:256-300),
// each ending in updateAbnormalEffect, which on a summon refreshes its
// appearance (Summon.updateAbnormalEffect); and Playable.doDie's blessing
// branch (Playable.java:148-164).

// blessPet puts the named blessing effect on pet and returns it.
func blessPet(t *testing.T, h *petWorld, pet *summon.Actor, skillID modelskill.ID, name string) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: skillID, Level: 1}, modelskill.EffectTemplate{Name: name, Time: 1800, Icon: true})
	if err != nil {
		t.Fatalf("effect.New(%s): %v", name, err)
	}
	e.Effector, e.Effected = pet, pet
	runOn(t, pet.Queue(), func() { pet.EffectList().Add(e) })
	drainUntilQuiet(t, h.client)
	return e
}

// petInfoCount counts the owner's PetInfo frames in frames up to the pet's
// Die (all frames when stopAtDie is false).
func petInfoCount(frames [][]byte, petID int32, stopAtDie bool) (count int, sawDie bool) {
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeDie:
			if wire.NewReader(f[1:]).ReadInt32() == petID {
				sawDie = true
				if stopAtDie {
					return count, sawDie
				}
			}
		case serverpackets.OpcodePetInfo:
			count++
		}
	}
	return count, sawDie
}

// TestBlessingEndingRefreshesPetAppearance ends a pet's Charm of Luck and
// Phoenix Blessing the way expiry and cancellation do: each ending refreshes
// the pet's appearance for its owner once; the list's icon update sends a
// PartySpelled, not a PetInfo.
func TestBlessingEndingRefreshesPetAppearance(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"CharmOfLuck", "PhoenixBless"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollar(t)
			pet, _ := h.spawnWolf(t)
			e := blessPet(t, h, pet, 438, name)

			runOn(t, pet.Queue(), func() { pet.EffectList().Remove(e) })
			const want = 1
			if got, _ := petInfoCount(drainFrames(t, h.client), pet.ObjectID(), false); got != want {
				t.Fatalf("owner PetInfo frames after %s ended = %d, want %d", name, got, want)
			}
		})
	}
}

// TestPhoenixBlessedPetDeathRefreshesCharmOfLuckTwice kills a Phoenix-blessed
// pet. Playable.doDie keeps the Phoenix Blessing and stops a Charm of Luck
// with stopCharmOfLuck(null): the charm's onExit refreshes the pet's
// appearance, and the outer stop refreshes it again, so the owner sees two
// refreshes before the pet's Die. Without a charm, no blessing stop runs and
// no refresh comes. The charm's icon update sends no PetInfo.
func TestPhoenixBlessedPetDeathRefreshesCharmOfLuckTwice(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		charm bool
		want  int
	}{
		{name: "phoenix only", want: 0},
		{name: "phoenix and charm of luck", charm: true, want: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollar(t)
			pet, _ := h.spawnWolf(t)
			blessPet(t, h, pet, 438, "PhoenixBless")
			if tt.charm {
				blessPet(t, h, pet, 2168, "CharmOfLuck")
			}

			killPetKeepingFrames(t, h, pet)
			got, sawDie := petInfoCount(drainFrames(t, h.client), pet.ObjectID(), true)
			if !sawDie {
				t.Fatal("owner never saw the pet's Die")
			}
			if got != tt.want {
				t.Fatalf("owner PetInfo frames before the pet's Die = %d, want %d", got, tt.want)
			}
			if !pet.EffectList().IsAffected(effect.FlagPhoenixBlessing) {
				t.Fatal("pet lost its Phoenix Blessing at death")
			}
			if pet.EffectList().IsAffected(effect.FlagCharmOfLuck) {
				t.Fatal("pet kept its Charm of Luck through a Phoenix-blessed death")
			}
		})
	}
}

package pets

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// farHostileAt is where the shift tests park a monster well out of the
// pet's reach.
var farHostileAt = location.Location{X: 600, Y: 20, Z: 30}

func encodeRequestActionUseShift(actionID int32, ctrl, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestActionUse)
	w.WriteInt32(actionID)
	w.WriteInt32(wire.BoolInt32(ctrl))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

// commandAt selects targetID at at and presses actionID with shift.
func (h *petWorld) commandAt(t *testing.T, targetID int32, at location.Location, actionID int32, shift bool) {
	t.Helper()
	h.client.Send(encodeAction(targetID, int32(at.X), int32(at.Y), int32(at.Z), false))
	drainFrames(t, h.client)
	h.client.Send(encodeRequestActionUseShift(actionID, false, shift))
	h.handled(t)
}

// requirePetHeldBack runs AI thinks with the owner standing still and fails
// if the pet came any closer to target or swung at it.
func (h *petWorld) requirePetHeldBack(t *testing.T, petActor *summon.Actor, target *npc.Hostile, why string) {
	t.Helper()
	tx, ty, _ := target.Position()
	dist := func() float64 {
		px, py, _ := petActor.Position()
		return math.Hypot(float64(tx-px), float64(ty-py))
	}
	before := dist()
	var frames [][]byte
	for range 3 {
		h.think(t)
		frames = append(frames, drainFrames(t, h.client)...)
	}
	if after := dist(); after < before {
		t.Fatalf("%s: pet closed from %.0f to %.0f on the target, want it held back", why, before, after)
	}
	if n := petAttacks(frames, petActor); n != 0 {
		t.Fatalf("%s: pet swung %d times, want none", why, n)
	}
}

// TestShiftAttackOnFarTargetGoesIdle presses the pet's attack with SHIFT on a
// monster out of its reach. The pet does not run at it: it goes idle, which
// for a pet following its owner means following the owner again, and for one
// told to stop following means standing where it is.
func TestShiftAttackOnFarTargetGoesIdle(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		follow bool
		want   summon.Intent
	}{
		{"following", true, summon.IntentFollowOwner},
		{"not following", false, summon.IntentIdle},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, petActor := bootFollowingPet(t)
			if !tc.follow {
				h.toggleFollow(t)
			}
			hostile := h.srv.SpawnHostileNPCAt(t, farHostileAt)
			drainUntilQuiet(t, h.client)

			h.commandAt(t, hostile.ObjectID(), farHostileAt, petAttackAction, true)
			if petActor.Move().Moving() {
				t.Fatal("pet started moving on a shift attack out of reach")
			}
			if got := petActor.Intent(); got != tc.want {
				t.Fatalf("pet intent = %v, want %v", got, tc.want)
			}
			h.requirePetHeldBack(t, petActor, hostile, "shift attack")
			drainUntilQuiet(t, h.client)
		})
	}
}

// TestShiftAttackInReachSwings presses the pet's attack with SHIFT on a
// monster already in its reach: SHIFT only keeps the pet from walking, so it
// swings as usual.
func TestShiftAttackInReachSwings(t *testing.T) {
	t.Parallel()
	h, petActor := bootFollowingPet(t)
	hostile := spawnTankyHostile(t, h)
	drainUntilQuiet(t, h.client)

	h.commandAt(t, hostile.ObjectID(), location.Location{X: hostileX, Y: hostileY, Z: hostileZ}, petAttackAction, true)
	h.srv.AdvanceUntil(t, "the pet's swing", petActor.IsAttackingNow)
	drainUntilQuiet(t, h.client)
}

// TestShiftMoveToOwnerHoldsPosition has the owner target itself and press
// move-to-target. Move-to drops follow mode and follows the creature, here
// the owner: unshifted, the pet walks after the owner as it moves off; with
// SHIFT held the follow never walks and the pet stays where it is.
func TestShiftMoveToOwnerHoldsPosition(t *testing.T) {
	t.Parallel()
	for _, shift := range []bool{true, false} {
		name := "unshifted"
		if shift {
			name = "shift"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, petActor := bootFollowingPet(t)
			x, y, z := h.srv.PlayerPosition(t, h.ownerID)
			h.commandAt(t, h.ownerID, location.Location{X: x, Y: y, Z: z}, moveToTargetAction, shift)
			if petActor.FollowActive() {
				t.Fatal("move-to left follow mode on")
			}
			if got := petActor.Intent(); got != summon.IntentFollowTarget {
				t.Fatalf("pet intent = %v, want follow-target", got)
			}
			if shift {
				h.requirePetStaysPut(t, petActor, "pet told to move to its owner with shift")
			} else {
				h.ownerWalksAway(t)
				h.requirePetCatchesUp(t, petActor, "pet told to move to its owner")
			}
			drainUntilQuiet(t, h.client)
		})
	}
}

// TestShiftStrikeOutOfRangeGoesIdle presses the pet's strike with SHIFT on a
// monster beyond its cast range. The pet neither walks into range nor casts:
// it goes idle and follows its owner again.
func TestShiftStrikeOutOfRangeGoesIdle(t *testing.T) {
	t.Parallel()
	strike := wolfStrike()
	strike.CastRange = 100
	h, petActor, _ := bootWolfStrikerWith(t, strike, gameservertest.WithAITask())
	hostile := h.srv.SpawnHostileNPCAt(t, farHostileAt)
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeAction(hostile.ObjectID(), int32(farHostileAt.X), int32(farHostileAt.Y), int32(farHostileAt.Z), false))
	drainFrames(t, h.client)
	h.client.Send(encodeRequestActionUseShift(wolfStrikeAction, false, true))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "strike ActionFailed")
	if _, ok := firstOpcode(frames, serverpackets.OpcodeMagicSkillUse); ok {
		t.Fatal("pet cast a shift strike out of range")
	}
	if petActor.Move().Moving() {
		t.Fatal("pet started moving on a shift strike out of range")
	}
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent = %v, want follow-owner", got)
	}
	h.requirePetHeldBack(t, petActor, hostile, "shift strike")
	if petActor.CastingNow() {
		t.Fatal("pet casting after a shift strike out of range")
	}
	drainUntilQuiet(t, h.client)
}

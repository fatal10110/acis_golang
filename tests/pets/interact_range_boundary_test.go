package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBlockedInteractRangeBoundaryIsExclusive pins the owned-pet interact
// gate that a blocked INTERACT arrival re-checks: the 150-unit interaction
// distance excludes its boundary, so an owner stopped exactly 150 units
// from its pet gets no status window, and one unit closer does.
func TestBlockedInteractRangeBoundaryIsExclusive(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		distance int
		want     bool
	}{
		{name: "exactly 150", distance: 150, want: false},
		{name: "149", distance: 149, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			geo := &gameservertest.GateGeo{}
			h := bootOwnerWithCollarAndGeo(t, geo)
			pet, _ := h.spawnWolf(t)
			placePet(t, pet, location.Location{X: 400, Y: 20, Z: 30})
			drainUntilQuiet(t, h.client)

			startOwnedPetApproach(t, h, pet)
			mover := h.srv.PlayerMove(t, h.ownerID)
			for i := 0; i < 2; i++ {
				if _, moving := mover.UpdatePosition(move.PositionUpdateInterval); !moving {
					t.Fatalf("UpdatePosition() tick %d moving = false, want the approach walk under way", i+1)
				}
			}
			stopped := mover.Position()
			placePet(t, pet, location.Location{X: stopped.X + tc.distance, Y: stopped.Y, Z: stopped.Z})
			geo.Block()
			if _, moving := mover.UpdatePosition(move.PositionUpdateInterval); moving {
				t.Fatal("UpdatePosition() moving = true after the path closed, want blocked stop")
			}

			frames := drainFrames(t, h.client)
			if !hasOpcode(frames, serverpackets.OpcodeActionFailed) {
				t.Fatalf("blocked INTERACT at %d missing ActionFailed: opcodes %x", tc.distance, frameOpcodes(frames))
			}
			if got := hasOpcode(frames, serverpackets.OpcodePetStatusShow); got != tc.want {
				t.Fatalf("blocked INTERACT at %d PetStatusShow = %v, want %v: opcodes %x", tc.distance, got, tc.want, frameOpcodes(frames))
			}
			if got := hasOpcode(frames, serverpackets.OpcodeStopMove); got != tc.want {
				t.Fatalf("blocked INTERACT at %d StopMove = %v, want %v: opcodes %x", tc.distance, got, tc.want, frameOpcodes(frames))
			}
		})
	}
}

// TestShiftInteractApproachBoundaryIsExclusive pins the owned-pet interact
// approach check on a shift-click, which never walks: exactly 100 units
// away is out of reach (ActionFailed only), 99 opens the status window.
// The 100 here is today's approach radius without either collision radius;
// #2796 adds the collision radii and will move this boundary.
func TestShiftInteractApproachBoundaryIsExclusive(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		distance int
		want     bool
	}{
		{name: "exactly 100", distance: 100, want: false},
		{name: "99", distance: 99, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollar(t)
			pet, _ := h.spawnWolf(t)
			px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
			placePet(t, pet, location.Location{X: px + tc.distance, Y: py, Z: pz})
			drainUntilQuiet(t, h.client)

			h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), false))
			drainUntilQuiet(t, h.client)
			h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), true))
			frames := drainFrames(t, h.client)
			if !hasOpcode(frames, serverpackets.OpcodeActionFailed) {
				t.Fatalf("shift INTERACT at %d missing ActionFailed: opcodes %x", tc.distance, frameOpcodes(frames))
			}
			if hasOpcode(frames, serverpackets.OpcodeMoveToLocation) {
				t.Fatalf("shift INTERACT at %d walked: opcodes %x", tc.distance, frameOpcodes(frames))
			}
			if got := hasOpcode(frames, serverpackets.OpcodePetStatusShow); got != tc.want {
				t.Fatalf("shift INTERACT at %d PetStatusShow = %v, want %v: opcodes %x", tc.distance, got, tc.want, frameOpcodes(frames))
			}
		})
	}
}

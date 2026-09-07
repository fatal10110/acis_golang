package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBlockedInteractInRangeBroadcastsStopMoveAndPetStatus pins the player
// INTERACT blocked-arrival arm when the target is inside interaction
// distance: ActionFailed, StopMove, then the pet status window. The base
// same-cell MoveToLocation correction must not fire.
func TestBlockedInteractInRangeBroadcastsStopMoveAndPetStatus(t *testing.T) {
	geo := &gameservertest.GateGeo{}
	h := bootOwnerWithCollarAndGeo(t, geo)
	pet, _ := h.spawnWolf(t)
	placePet(t, pet, location.Location{X: 130, Y: 20, Z: 30})
	drainUntilQuiet(t, h.client)

	startOwnedPetApproach(t, h, pet)
	h.srv.TickPlayerBlocked(t, h.ownerID, geo)

	frames := drainFrames(t, h.client)
	if hasOpcode(frames, serverpackets.OpcodeMoveToLocation) {
		t.Fatalf("in-range INTERACT blocked sent MoveToLocation, want StopMove: opcodes %x", frameOpcodes(frames))
	}
	if !hasOpcode(frames, serverpackets.OpcodeActionFailed) {
		t.Fatalf("in-range INTERACT blocked missing ActionFailed: opcodes %x", frameOpcodes(frames))
	}
	if !hasOpcode(frames, serverpackets.OpcodeStopMove) {
		t.Fatalf("in-range INTERACT blocked missing StopMove: opcodes %x", frameOpcodes(frames))
	}
	if !hasOpcode(frames, serverpackets.OpcodePetStatusShow) {
		t.Fatalf("in-range INTERACT blocked missing PetStatusShow: opcodes %x", frameOpcodes(frames))
	}
}

// TestBlockedInteractOutOfRangeBroadcastsMoveToLocation pins the INTERACT
// blocked-arrival arm when the target is still too far: ActionFailed plus
// the base same-cell MoveToLocation, and no pet window.
func TestBlockedInteractOutOfRangeBroadcastsMoveToLocation(t *testing.T) {
	geo := &gameservertest.GateGeo{}
	h := bootOwnerWithCollarAndGeo(t, geo)
	pet, _ := h.spawnWolf(t)
	placePet(t, pet, location.Location{X: 220, Y: 20, Z: 30})
	drainUntilQuiet(t, h.client)

	startOwnedPetApproach(t, h, pet)
	advanced := h.srv.TickPlayerBlocked(t, h.ownerID, geo)

	frames := drainFrames(t, h.client)
	if hasOpcode(frames, serverpackets.OpcodeStopMove) {
		t.Fatalf("out-of-range INTERACT blocked sent StopMove, want MoveToLocation: opcodes %x", frameOpcodes(frames))
	}
	if hasOpcode(frames, serverpackets.OpcodePetStatusShow) {
		t.Fatalf("out-of-range INTERACT blocked sent PetStatusShow: opcodes %x", frameOpcodes(frames))
	}
	if !hasOpcode(frames, serverpackets.OpcodeActionFailed) {
		t.Fatalf("out-of-range INTERACT blocked missing ActionFailed: opcodes %x", frameOpcodes(frames))
	}
	frame, ok := firstOpcode(frames, serverpackets.OpcodeMoveToLocation)
	if !ok {
		t.Fatalf("out-of-range INTERACT blocked missing MoveToLocation: opcodes %x", frameOpcodes(frames))
	}
	objectID, dest, origin := gameservertest.ReadMoveToLocationCoords(t, frame)
	if objectID != h.ownerID {
		t.Fatalf("MoveToLocation object id = %d, want %d", objectID, h.ownerID)
	}
	if dest != advanced || origin != advanced {
		t.Fatalf("MoveToLocation dest/origin = %+v/%+v, want advanced cell %+v", dest, origin, advanced)
	}
}

// TestBlockedWalkAfterPetApproachBroadcastsMoveToLocation pins that a later
// client walk replaces the parked INTERACT slot: blocked arrival is MOVE_TO
// (same-cell MoveToLocation), not StopMove + PetStatusShow.
func TestBlockedWalkAfterPetApproachBroadcastsMoveToLocation(t *testing.T) {
	geo := &gameservertest.GateGeo{}
	h := bootOwnerWithCollarAndGeo(t, geo)
	pet, _ := h.spawnWolf(t)
	placePet(t, pet, location.Location{X: 220, Y: 20, Z: 30})
	drainUntilQuiet(t, h.client)

	startOwnedPetApproach(t, h, pet)
	h.client.Send(encodeMoveBackwardToLocation(80, 20, 30))
	readUntilOpcode(t, h.client, serverpackets.OpcodeMoveToLocation, "walk MoveToLocation")
	drainUntilQuiet(t, h.client)

	advanced := h.srv.TickPlayerBlocked(t, h.ownerID, geo)
	frames := drainFrames(t, h.client)
	if hasOpcode(frames, serverpackets.OpcodeStopMove) {
		t.Fatalf("MOVE_TO after INTERACT approach sent StopMove: opcodes %x", frameOpcodes(frames))
	}
	if hasOpcode(frames, serverpackets.OpcodePetStatusShow) {
		t.Fatalf("MOVE_TO after INTERACT approach sent PetStatusShow: opcodes %x", frameOpcodes(frames))
	}
	if hasOpcode(frames, serverpackets.OpcodeActionFailed) {
		t.Fatalf("MOVE_TO after INTERACT approach sent ActionFailed: opcodes %x", frameOpcodes(frames))
	}
	frame, ok := firstOpcode(frames, serverpackets.OpcodeMoveToLocation)
	if !ok {
		t.Fatalf("MOVE_TO after INTERACT approach missing MoveToLocation: opcodes %x", frameOpcodes(frames))
	}
	objectID, dest, origin := gameservertest.ReadMoveToLocationCoords(t, frame)
	if objectID != h.ownerID {
		t.Fatalf("MoveToLocation object id = %d, want %d", objectID, h.ownerID)
	}
	if dest != advanced || origin != advanced {
		t.Fatalf("MoveToLocation dest/origin = %+v/%+v, want advanced cell %+v", dest, origin, advanced)
	}
}

// TestBlockedInteractAfterPetReturnStillStopMoveAndPetStatus pins that an
// INTERACT blocked arrival whose pet has already left the world still uses
// the in-range arm: ActionFailed, StopMove, PetStatusShow — not the base
// same-cell MoveToLocation. The last-known pet cell is still inside
// interaction range, so the interact gate passes and onInteract has no
// world-presence check.
func TestBlockedInteractAfterPetReturnStillStopMoveAndPetStatus(t *testing.T) {
	geo := &gameservertest.GateGeo{}
	h := bootOwnerWithCollarAndGeo(t, geo)
	pet, _ := h.spawnWolf(t)
	placePet(t, pet, location.Location{X: 130, Y: 20, Z: 30})
	drainUntilQuiet(t, h.client)

	startOwnedPetApproach(t, h, pet)
	h.returnPet(t)
	h.srv.TickPlayerBlocked(t, h.ownerID, geo)

	frames := drainFrames(t, h.client)
	if hasOpcode(frames, serverpackets.OpcodeMoveToLocation) {
		t.Fatalf("returned-pet INTERACT blocked sent MoveToLocation, want StopMove: opcodes %x", frameOpcodes(frames))
	}
	if !hasOpcode(frames, serverpackets.OpcodeActionFailed) {
		t.Fatalf("returned-pet INTERACT blocked missing ActionFailed: opcodes %x", frameOpcodes(frames))
	}
	if !hasOpcode(frames, serverpackets.OpcodeStopMove) {
		t.Fatalf("returned-pet INTERACT blocked missing StopMove: opcodes %x", frameOpcodes(frames))
	}
	if !hasOpcode(frames, serverpackets.OpcodePetStatusShow) {
		t.Fatalf("returned-pet INTERACT blocked missing PetStatusShow: opcodes %x", frameOpcodes(frames))
	}
}

// TestBlockedAttackAfterPetApproachBroadcastsMoveToLocation pins that an
// attack chase replaces the parked INTERACT slot: blocked arrival is ATTACK
// (same-cell MoveToLocation), not StopMove + PetStatusShow.
func TestBlockedAttackAfterPetApproachBroadcastsMoveToLocation(t *testing.T) {
	geo := &gameservertest.GateGeo{}
	h := bootOwnerWithCollarAndGeo(t, geo)
	pet, _ := h.spawnWolf(t)
	placePet(t, pet, location.Location{X: 220, Y: 20, Z: 30})
	drainUntilQuiet(t, h.client)

	startOwnedPetApproach(t, h, pet)
	startDistantAttackChase(t, h)

	advanced := h.srv.TickPlayerBlocked(t, h.ownerID, geo)
	frames := drainFrames(t, h.client)
	if hasOpcode(frames, serverpackets.OpcodeStopMove) {
		t.Fatalf("ATTACK after INTERACT approach sent StopMove: opcodes %x", frameOpcodes(frames))
	}
	if hasOpcode(frames, serverpackets.OpcodePetStatusShow) {
		t.Fatalf("ATTACK after INTERACT approach sent PetStatusShow: opcodes %x", frameOpcodes(frames))
	}
	if hasOpcode(frames, serverpackets.OpcodeActionFailed) {
		t.Fatalf("ATTACK after INTERACT approach sent ActionFailed: opcodes %x", frameOpcodes(frames))
	}
	frame, ok := firstOpcode(frames, serverpackets.OpcodeMoveToLocation)
	if !ok {
		t.Fatalf("ATTACK after INTERACT approach missing MoveToLocation: opcodes %x", frameOpcodes(frames))
	}
	objectID, dest, origin := gameservertest.ReadMoveToLocationCoords(t, frame)
	if objectID != h.ownerID {
		t.Fatalf("MoveToLocation object id = %d, want %d", objectID, h.ownerID)
	}
	if dest != advanced || origin != advanced {
		t.Fatalf("MoveToLocation dest/origin = %+v/%+v, want advanced cell %+v", dest, origin, advanced)
	}
}

func bootOwnerWithCollarAndGeo(t *testing.T, geo move.Geo) *petWorld {
	t.Helper()
	srv := bootPets(t, gameservertest.WithGeo(geo))
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)
	return h
}

func placePet(t *testing.T, pet *summon.Actor, at location.Location) {
	t.Helper()
	pet.SyncPosition(at)
}

func startOwnedPetApproach(t *testing.T, h *petWorld, pet *summon.Actor) {
	t.Helper()
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), false))
	assertFrameOpcode(t, mustRead(t, h.client, "interact ActionFailed"), serverpackets.OpcodeActionFailed, "interact ActionFailed")
	assertFrameOpcode(t, mustRead(t, h.client, "approach MoveToLocation"), serverpackets.OpcodeMoveToLocation, "approach MoveToLocation")
	drainUntilQuiet(t, h.client)
}

func startDistantAttackChase(t *testing.T, h *petWorld) {
	t.Helper()
	hostile := h.srv.SpawnHostileNPCAt(t, location.Location{X: 600, Y: 20, Z: 30})
	drainUntilQuiet(t, h.client)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, h.client)
}

func hasOpcode(frames [][]byte, opcode byte) bool {
	_, ok := firstOpcode(frames, opcode)
	return ok
}

func firstOpcode(frames [][]byte, opcode byte) ([]byte, bool) {
	for _, frame := range frames {
		if len(frame) > 0 && frame[0] == opcode {
			return frame, true
		}
	}
	return nil, false
}

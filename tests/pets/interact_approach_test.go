package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// ownedPetInteractReach is how close an owner must stand to its wolf for a
// click to open the status window without walking: the 100-unit approach
// offset plus the male human fighter's 9-unit body and the wolf's 8-unit
// body, all strict.
const ownedPetInteractReach = 100 + 9 + 8

// bootBodiedOwner is bootOwnerWithCollar with the owner's class carrying the
// datapack human fighter body (radius 9 male, 8 female), which the shared
// class template leaves at zero.
func bootBodiedOwner(t *testing.T) *petWorld {
	t.Helper()
	tmpl := gameservertest.ClassTemplate()
	tmpl.CollisionRadius, tmpl.CollisionHeight = 9, 23
	tmpl.CollisionRadiusFemale, tmpl.CollisionHeightFemale = 8, 23.5
	return bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithClassTemplate(tmpl)})
}

// interactReach reads both live bodies and checks them against the fixed
// reach, so a fixture change shows up here rather than as a moved boundary.
func interactReach(t *testing.T, h *petWorld, pet *summon.Actor) int {
	t.Helper()
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner not in world")
	}
	owner, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("owner %T is not a combatant", obj)
	}
	if got := int(100 + owner.CollisionRadius() + pet.CollisionRadius()); got != ownedPetInteractReach {
		t.Fatalf("owner radius %v + pet radius %v give reach %d, want %d", owner.CollisionRadius(), pet.CollisionRadius(), got, ownedPetInteractReach)
	}
	return ownedPetInteractReach
}

// moveToPawnFields decodes a MoveToPawn frame: mover, target, stop
// distance and the mover's position.
func moveToPawnFields(t *testing.T, frame []byte) (objectID, targetID int32, distance int32, at location.Location) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMoveToPawn, "MoveToPawn")
	r := wire.NewReader(frame[1:])
	objectID, targetID, distance = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	at = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	return objectID, targetID, distance, at
}

// clickOwnedPet selects pet, drains the selection, then clicks it again.
func clickOwnedPet(t *testing.T, h *petWorld, pet *summon.Actor, shift bool) [][]byte {
	t.Helper()
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), shift))
	return drainFrames(t, h.client)
}

// TestOwnedPetInteractReachIncludesCollisionRadii pins the click that opens
// the status window at once: one unit inside the approach offset plus both
// bodies, the owner does not walk but is shown facing its pet from the
// interaction distance before the window opens.
func TestOwnedPetInteractReachIncludesCollisionRadii(t *testing.T) {
	t.Parallel()
	h := bootBodiedOwner(t)
	pet, _ := h.spawnWolf(t)
	reach := interactReach(t, h, pet)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	placePet(t, pet, location.Location{X: px + reach - 1, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)

	frames := clickOwnedPet(t, h, pet, false)
	if hasOpcode(frames, serverpackets.OpcodeMoveToLocation) {
		t.Fatalf("click at %d walked: opcodes %x", reach-1, frameOpcodes(frames))
	}
	if h.srv.PlayerMove(t, h.ownerID).Moving() {
		t.Fatalf("click at %d left the owner moving", reach-1)
	}
	var order []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodePetStatusShow:
			order = append(order, f[0])
		}
	}
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodePetStatusShow}
	if string(order) != string(want) {
		t.Fatalf("click at %d order = %x, want %x (all opcodes %x)", reach-1, order, want, frameOpcodes(frames))
	}
	frame, _ := firstOpcode(frames, serverpackets.OpcodeMoveToPawn)
	objectID, targetID, distance, at := moveToPawnFields(t, frame)
	if objectID != h.ownerID || targetID != pet.ObjectID() || distance != 150 {
		t.Fatalf("MoveToPawn = mover %d target %d distance %d, want %d/%d/150", objectID, targetID, distance, h.ownerID, pet.ObjectID())
	}
	if want := (location.Location{X: px, Y: py, Z: pz}); at != want {
		t.Fatalf("MoveToPawn position = %+v, want owner position %+v", at, want)
	}
}

// TestOwnedPetInteractOutOfReachWalksByPawn pins the approach a click at
// the reach boundary starts: a MoveToPawn toward the pet stopping 100 units
// short, not a MoveToLocation onto the pet's cell, and no status window yet.
func TestOwnedPetInteractOutOfReachWalksByPawn(t *testing.T) {
	t.Parallel()
	h := bootBodiedOwner(t)
	pet, _ := h.spawnWolf(t)
	reach := interactReach(t, h, pet)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	placePet(t, pet, location.Location{X: px + reach, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)

	frames := clickOwnedPet(t, h, pet, false)
	if hasOpcode(frames, serverpackets.OpcodeMoveToLocation) {
		t.Fatalf("approach sent MoveToLocation: opcodes %x", frameOpcodes(frames))
	}
	if hasOpcode(frames, serverpackets.OpcodePetStatusShow) {
		t.Fatalf("click at %d opened the status window before walking: opcodes %x", reach, frameOpcodes(frames))
	}
	if !hasOpcode(frames, serverpackets.OpcodeActionFailed) {
		t.Fatalf("approach missing ActionFailed: opcodes %x", frameOpcodes(frames))
	}
	frame, ok := firstOpcode(frames, serverpackets.OpcodeMoveToPawn)
	if !ok {
		t.Fatalf("approach missing MoveToPawn: opcodes %x", frameOpcodes(frames))
	}
	objectID, targetID, distance, at := moveToPawnFields(t, frame)
	if objectID != h.ownerID || targetID != pet.ObjectID() || distance != 100 {
		t.Fatalf("approach MoveToPawn = mover %d target %d distance %d, want %d/%d/100", objectID, targetID, distance, h.ownerID, pet.ObjectID())
	}
	if want := (location.Location{X: px, Y: py, Z: pz}); at != want {
		t.Fatalf("approach MoveToPawn position = %+v, want owner position %+v", at, want)
	}
}

// TestOwnedPetInteractApproachArrivalOpensStatus pins the arrival of the
// approach walk: the interact is thought again, so the owner is released,
// shown facing its pet from the interaction distance, and gets the window.
func TestOwnedPetInteractApproachArrivalOpensStatus(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	placePet(t, pet, location.Location{X: px + 300, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)

	frames := clickOwnedPet(t, h, pet, false)
	if !hasOpcode(frames, serverpackets.OpcodeMoveToPawn) || hasOpcode(frames, serverpackets.OpcodePetStatusShow) {
		t.Fatalf("click at 300 = opcodes %x, want an approach and no window", frameOpcodes(frames))
	}
	mover := h.srv.PlayerMove(t, h.ownerID)
	h.srv.AdvanceUntil(t, "approach arrival", func() bool { return !mover.Moving() })
	h.srv.Advance(t, 100*time.Millisecond)

	frames = drainFrames(t, h.client)
	var order []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodePetStatusShow:
			order = append(order, f[0])
		}
	}
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodePetStatusShow}
	if string(order) != string(want) {
		t.Fatalf("arrival order = %x, want %x (all opcodes %x)", order, want, frameOpcodes(frames))
	}
	frame, _ := firstOpcode(frames, serverpackets.OpcodeMoveToPawn)
	if _, targetID, distance, _ := moveToPawnFields(t, frame); targetID != pet.ObjectID() || distance != 150 {
		t.Fatalf("arrival MoveToPawn target %d distance %d, want %d/150", targetID, distance, pet.ObjectID())
	}
}

// TestOwnedPetInteractApproachRerunsAfterWeaponToggle pins a sword put on
// mid-way into the approach walk (#2877): PlayerAI.thinkUseItem
// (PlayerAI.java:505-519) toggles it, then runs the INTERACT it replaced
// again (doIntention(_previousIntention)), whose thinkInteract
// (PlayerAI.java:413-462) answers ActionFailed and walks toward the pet
// afresh with MoveToPawn. The arrival still opens the status window.
func TestOwnedPetInteractApproachRerunsAfterWeaponToggle(t *testing.T) {
	t.Parallel()
	const swordTemplateID = int32(30)
	h := bootOwnerWithCollar(t, seedItem{swordTemplateID, 1})
	sword := h.seededItem(t, swordTemplateID)
	pet, _ := h.spawnWolf(t)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	placePet(t, pet, location.Location{X: px + 300, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)

	frames := clickOwnedPet(t, h, pet, false)
	if !hasOpcode(frames, serverpackets.OpcodeMoveToPawn) {
		t.Fatalf("click at 300 = opcodes %x, want an approach", frameOpcodes(frames))
	}
	h.client.Send(encodeUseItem(sword, false))
	frames = drainFrames(t, h.client)
	var order []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSystemMessage:
			if wire.NewReader(f[1:]).ReadInt32() == serverpackets.SystemMessageS1Equipped {
				order = append(order, f[0])
			}
		case serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodePetStatusShow:
			order = append(order, f[0])
		}
	}
	want := []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn}
	if string(order) != string(want) {
		t.Fatalf("mid-approach toggle order = %x, want S1_EQUIPPED, ActionFailed, MoveToPawn (all opcodes %x)", order, frameOpcodes(frames))
	}
	frame, _ := firstOpcode(frames, serverpackets.OpcodeMoveToPawn)
	if objectID, targetID, distance, _ := moveToPawnFields(t, frame); objectID != h.ownerID || targetID != pet.ObjectID() || distance != 100 {
		t.Fatalf("fresh approach MoveToPawn = mover %d target %d distance %d, want %d/%d/100", objectID, targetID, distance, h.ownerID, pet.ObjectID())
	}

	mover := h.srv.PlayerMove(t, h.ownerID)
	h.srv.AdvanceUntil(t, "approach arrival", func() bool { return !mover.Moving() })
	h.srv.Advance(t, 100*time.Millisecond)
	if !hasOpcode(drainFrames(t, h.client), serverpackets.OpcodePetStatusShow) {
		t.Fatal("the re-run approach arrived without opening the status window")
	}
}

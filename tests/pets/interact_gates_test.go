package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// interactOpcodes keeps the interact answer's opcodes in order:
// ActionFailed, MoveToPawn and PetStatusShow.
func interactOpcodes(frames [][]byte) []byte {
	var order []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodePetStatusShow:
			order = append(order, f[0])
		}
	}
	return order
}

func encodeRequestChangeWaitType(stand bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeWaitType)
	w.WriteInt32(wire.BoolInt32(stand))
	return w.Bytes()
}

// sitOwner seats the owner through RequestChangeWaitType.
func sitOwner(t *testing.T, h *petWorld) {
	t.Helper()
	h.client.Send(encodeRequestChangeWaitType(false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeChangeWaitType, "sit ChangeWaitType")
	h.srv.Advance(t, 3*time.Second)
	drainUntilQuiet(t, h.client)
}

// rootOwner lands a real Root effect on the owner on its own queue.
func rootOwner(t *testing.T, h *petWorld) {
	t.Helper()
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	owner, ok := obj.(interface {
		effect.Actor
		EffectList() *effect.List
		Queue() *sim.Queue
		MovementDisabled() bool
	})
	if !ok {
		t.Fatalf("world player %T is not an effect holder", obj)
	}
	e, err := effect.New(effect.Skill{ID: 102, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Root", Time: 30})
	if err != nil {
		t.Fatalf("effect.New(Root): %v", err)
	}
	e.Effector, e.Effected = owner, owner
	done := make(chan struct{})
	if !owner.Queue().Post(func() { owner.EffectList().Add(e); close(done) }) {
		t.Fatal("post root: queue closed")
	}
	<-done
	if !owner.MovementDisabled() {
		t.Fatal("Root left the owner's movement enabled")
	}
	drainUntilQuiet(t, h.client)
}

// startOwnerTrade opens a direct trade between the owner and a second
// player, leaving the owner with an active trade.
func startOwnerTrade(t *testing.T, h *petWorld) {
	t.Helper()
	buyerID := h.srv.SeedCharacterFor(t, "player2", "Buyer", 1, 0).ID
	buyer := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, buyer)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, buyer)
	h.client.Send(encodeTradeRequest(buyerID))
	drainUntilQuiet(t, buyer)
	buyer.Send(encodeAnswerTradeRequest(1))
	readUntilOpcode(t, h.client, serverpackets.OpcodeTradeStart, "owner TradeStart")
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, buyer)
}

// requireOnlyActionFailed checks an interact answered by ActionFailed
// alone: no approach, no facing packet, no status window, and the owner
// still where it stood.
func requireOnlyActionFailed(t *testing.T, h *petWorld, frames [][]byte, from location.Location, what string) {
	t.Helper()
	if got := interactOpcodes(frames); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("%s: interact opcodes = %x, want ActionFailed only (all opcodes %x)", what, got, frameOpcodes(frames))
	}
	if hasOpcode(frames, serverpackets.OpcodeMoveToLocation) {
		t.Fatalf("%s: walked with MoveToLocation: opcodes %x", what, frameOpcodes(frames))
	}
	h.srv.Advance(t, time.Second)
	if h.srv.PlayerMove(t, h.ownerID).Moving() {
		t.Fatalf("%s: owner is moving", what)
	}
	if x, y, z := h.srv.PlayerPosition(t, h.ownerID); (location.Location{X: x, Y: y, Z: z}) != from {
		t.Fatalf("%s: owner moved from %+v to %d,%d,%d", what, from, x, y, z)
	}
	if extra := drainFrames(t, h.client); hasOpcode(extra, serverpackets.OpcodePetStatusShow) || hasOpcode(extra, serverpackets.OpcodeMoveToPawn) {
		t.Fatalf("%s: late interact frames %x", what, frameOpcodes(extra))
	}
}

// TestOwnedPetInteractGatesInReach pins the gates the interact think runs
// before the reach check: an owner with an active trade, or seated, who
// clicks its pet in reach gets ActionFailed alone and no status window.
func TestOwnedPetInteractGatesInReach(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		gate func(*testing.T, *petWorld)
	}{
		{"trading", startOwnerTrade},
		{"seated", sitOwner},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollar(t)
			pet, _ := h.spawnWolf(t)
			px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
			placePet(t, pet, location.Location{X: px + 50, Y: py, Z: pz})
			drainUntilQuiet(t, h.client)
			tc.gate(t, h)

			frames := clickOwnedPet(t, h, pet, false)
			requireOnlyActionFailed(t, h, frames, location.Location{X: px, Y: py, Z: pz}, tc.name+" owner clicking its pet at 50")
		})
	}
}

// TestOwnedPetInteractGatesOutOfReach pins the walk gates: a seated owner
// stops at the sit gate, and a rooted owner out of reach gets no approach.
// Neither walks nor gets a status window.
func TestOwnedPetInteractGatesOutOfReach(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		gate func(*testing.T, *petWorld)
	}{
		{"seated", sitOwner},
		{"rooted", rootOwner},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollar(t)
			pet, _ := h.spawnWolf(t)
			px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
			placePet(t, pet, location.Location{X: px + 300, Y: py, Z: pz})
			drainUntilQuiet(t, h.client)
			tc.gate(t, h)

			frames := clickOwnedPet(t, h, pet, false)
			requireOnlyActionFailed(t, h, frames, location.Location{X: px, Y: py, Z: pz}, tc.name+" owner clicking its pet at 300")
		})
	}
}

// TestOwnedPetInteractRootedInReachOpensStatus pins that a root only stops
// the approach walk: a rooted owner already in reach still faces its pet
// and gets the status window.
func TestOwnedPetInteractRootedInReachOpensStatus(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	placePet(t, pet, location.Location{X: px + 50, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)
	rootOwner(t, h)

	frames := clickOwnedPet(t, h, pet, false)
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodePetStatusShow}
	if got := interactOpcodes(frames); string(got) != string(want) {
		t.Fatalf("rooted click at 50 = %x, want %x (all opcodes %x)", got, want, frameOpcodes(frames))
	}
}

// TestOwnedPetInteractApproachFollowsMovedPet moves the pet out of reach of
// the approach's destination while the owner walks. The approach is a
// tracking pawn walk (PlayerMove.updatePosition re-aims every update at
// where the pawn stands, PlayerMove.java:234): it follows the pet and ends
// within 100 of its new cell, even with no position update running and
// only the walk's arrival timer ending it. That arrival opens the window;
// no second approach is sent.
func TestOwnedPetInteractApproachFollowsMovedPet(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarAndGeo(t, gameservertest.FlatGeo{Z: gameservertest.SpawnZ})
	pet, _ := h.spawnWolf(t)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	placePet(t, pet, location.Location{X: px + 300, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)

	frames := clickOwnedPet(t, h, pet, false)
	if !hasOpcode(frames, serverpackets.OpcodeMoveToPawn) || hasOpcode(frames, serverpackets.OpcodePetStatusShow) {
		t.Fatalf("click at 300 = opcodes %x, want an approach and no window", frameOpcodes(frames))
	}
	moved := location.Location{X: px + 700, Y: py, Z: pz}
	placePet(t, pet, moved)
	mover := h.srv.PlayerMove(t, h.ownerID)
	h.srv.AdvanceUntil(t, "approach arrival", func() bool { return !mover.Moving() })
	h.srv.Advance(t, 100*time.Millisecond)

	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	if at := (location.Location{X: x, Y: y, Z: z}); !at.In2DRadius(moved, 100) || at.In2DRadius(moved, 80) {
		t.Fatalf("approach ended at %+v, %.1f from the moved pet; want the first step within 100", at, at.Distance2D(moved))
	}
	frames = drainFrames(t, h.client)
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodePetStatusShow}
	if got := interactOpcodes(frames); string(got) != string(want) {
		t.Fatalf("arrival at the moved pet = %x, want %x (all opcodes %x)", got, want, frameOpcodes(frames))
	}
	frame, _ := firstOpcode(frames, serverpackets.OpcodeMoveToPawn)
	if _, targetID, distance, _ := moveToPawnFields(t, frame); targetID != pet.ObjectID() || distance != 150 {
		t.Fatalf("arrival MoveToPawn target %d distance %d, want %d/150 (the facing, not a second approach)", targetID, distance, pet.ObjectID())
	}
}

// TestOwnedPetInteractArrivalAfterUnsummonFails unsummons the pet while its
// owner walks to it. Arrival finds the interact target gone and answers
// ActionFailed alone: no new approach and no status window.
func TestOwnedPetInteractArrivalAfterUnsummonFails(t *testing.T) {
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
	runOnPetQueue(t, pet, pet.Unsummon)
	readUntilOpcode(t, h.client, serverpackets.OpcodeDeleteObject, "pet DeleteObject")
	drainUntilQuiet(t, h.client)
	if _, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatal("pet still in world state after unsummon")
	}
	if !mover.Moving() {
		t.Fatal("unsummon stopped the owner's approach before arrival")
	}

	h.srv.AdvanceUntil(t, "approach arrival", func() bool { return !mover.Moving() })
	h.srv.Advance(t, 100*time.Millisecond)
	frames = drainFrames(t, h.client)
	if got := interactOpcodes(frames); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("arrival after unsummon = %x, want ActionFailed only (all opcodes %x)", got, frameOpcodes(frames))
	}
	if mover.Moving() {
		t.Fatal("owner walks again toward an unsummoned pet")
	}
}

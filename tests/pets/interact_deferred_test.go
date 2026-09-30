package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// readImmediate collects the frames already on their way to c, letting at
// most a millisecond pass on the driven clock: nothing a swing or cast in
// flight schedules comes due meanwhile.
func readImmediate(c *testsupport.ScriptedClient) [][]byte {
	var frames [][]byte
	for frame := c.ReadWithTimeout(time.Millisecond); frame != nil; frame = c.ReadWithTimeout(time.Millisecond) {
		frames = append(frames, frame)
	}
	return frames
}

// isOwnerInteractFrame reports whether frame is where the owner's interact
// with pet shows: the owner's MoveToPawn toward it (an approach walk, or the
// facing that precedes the window) or the status window itself.
func isOwnerInteractFrame(frame []byte, ownerID int32, pet *summon.Actor) bool {
	switch frame[0] {
	case serverpackets.OpcodePetStatusShow:
		return true
	case serverpackets.OpcodeMoveToPawn:
		r := wire.NewReader(frame[1:])
		return r.ReadInt32() == ownerID && r.ReadInt32() == pet.ObjectID()
	}
	return false
}

// clickPetQueued clicks the already-selected pet and checks the click is
// answered with ActionFailed alone while the action in flight holds it.
func clickPetQueued(t *testing.T, h *petWorld, pet *summon.Actor, what string) {
	t.Helper()
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), false))
	frames := readImmediate(h.client)
	if !hasOpcode(frames, serverpackets.OpcodeActionFailed) {
		t.Fatalf("pet click %s = opcodes %x, want ActionFailed", what, frameOpcodes(frames))
	}
	for _, f := range frames {
		if isOwnerInteractFrame(f, h.ownerID, pet) {
			t.Fatalf("pet click %s interacted at once: opcodes %x", what, frameOpcodes(frames))
		}
	}
}

// queuePetInteractMidSwing summons the owner's wolf next to it, starts the
// owner swinging at an adjacent monster, and clicks the pet twice while the
// first swing is in flight: once to select it, then the interact the swing
// holds queued.
func queuePetInteractMidSwing(t *testing.T) (*petWorld, *summon.Actor) {
	t.Helper()
	h := bootOwnerWithCollar(t)
	if !h.srv.DrivesClock() {
		t.Skip("holding a swing open needs the driven clock")
	}
	pet, _ := h.spawnWolf(t)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	placePet(t, pet, location.Location{X: px - 50, Y: py, Z: pz})
	hostile := h.srv.SpawnHostileNPCAt(t, location.Location{X: px + 30, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	for {
		frame := mustRead(t, h.client, "owner swing")
		if frame[0] == serverpackets.OpcodeAttack && wire.NewReader(frame[1:]).ReadInt32() == h.ownerID {
			break
		}
	}

	// Selecting the pet leaves the swing and the attack running.
	h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), false))
	readImmediate(h.client)
	clickPetQueued(t, h, pet, "mid-swing")
	return h, pet
}

// TestOwnedPetInteractMidSwingRunsAtSwingEnd pins PlayableAI.tryToInteract
// (PlayableAI.java:373-390) for a swing in flight: the owner's click on its
// pet is answered ActionFailed and kept as the next intention, and
// PlayableAI.onEvtFinishedAttack runs it once the swing ends, in place of
// the attack, which does not swing again.
func TestOwnedPetInteractMidSwingRunsAtSwingEnd(t *testing.T) {
	t.Parallel()
	h, pet := queuePetInteractMidSwing(t)

	for i := 0; ; i++ {
		frame := h.client.ReadWithTimeout(5 * time.Second)
		if frame == nil || i == 100 {
			t.Fatal("the queued interact never ran")
		}
		if frame[0] == serverpackets.OpcodeAttack && wire.NewReader(frame[1:]).ReadInt32() == h.ownerID {
			t.Fatal("the owner swung again before the queued interact ran")
		}
		if isOwnerInteractFrame(frame, h.ownerID, pet) {
			break
		}
	}
	for end := h.client.Now().Add(3 * time.Second); h.client.Now().Before(end); {
		frame := h.client.ReadWithTimeout(end.Sub(h.client.Now()))
		if frame == nil {
			break
		}
		if frame[0] == serverpackets.OpcodeAttack && wire.NewReader(frame[1:]).ReadInt32() == h.ownerID {
			t.Fatal("the owner swung again after the queued interact replaced the attack")
		}
	}
}

// TestOwnedPetInteractMidCastRunsAtCastEnd is the same for a cast in
// flight: PlayerAI.onEvtFinishedCasting runs the queued interact once the
// cast launches, and only then is the owner shown facing its pet and given
// the status window.
func TestOwnedPetInteractMidCastRunsAtCastEnd(t *testing.T) {
	t.Parallel()
	h := bootSummoner(t)
	if !h.srv.DrivesClock() {
		t.Skip("holding a cast open needs the driven clock")
	}
	pet, _ := h.spawnWolf(t)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	placePet(t, pet, location.Location{X: px + 50, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeRequestMagicSkillUse(longCastSkillID))
	readUntilOpcode(t, h.client, serverpackets.OpcodeMagicSkillUse, "long cast MagicSkillUse")
	readImmediate(h.client)
	if !h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatal("long cast not in flight")
	}
	clickPetQueued(t, h, pet, "mid-cast")

	frames := readUntilOpcode(t, h.client, serverpackets.OpcodePetStatusShow, "queued interact PetStatusShow")
	var order []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeMagicSkillLaunched, serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodePetStatusShow:
			order = append(order, f[0])
		}
	}
	want := []byte{serverpackets.OpcodeMagicSkillLaunched, serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodePetStatusShow}
	if string(order) != string(want) {
		t.Fatalf("cast end order = %x, want %x (all opcodes %x)", order, want, frameOpcodes(frames))
	}
}

// TestOwnedPetInteractQueuedForReturnedPetEndsTheAttack: a pet sent back to
// its collar while the owner's interact with it waits behind a swing is a
// lost target when the swing ends. PlayerAI.thinkInteract
// (PlayerAI.java:413-428) releases the click with ActionFailed and goes idle
// (AbstractAI.isTargetLost, AbstractAI.java:586-595): no approach, no status
// window, and no further swing at the monster.
func TestOwnedPetInteractQueuedForReturnedPetEndsTheAttack(t *testing.T) {
	t.Parallel()
	h, pet := queuePetInteractMidSwing(t)

	h.client.Send(encodeRequestActionUse(19, false))
	for {
		frame := mustRead(t, h.client, "PetDelete")
		if frame[0] == serverpackets.OpcodePetDelete {
			break
		}
		if frame[0] == serverpackets.OpcodeAttack && wire.NewReader(frame[1:]).ReadInt32() == h.ownerID {
			t.Fatal("the swing ended before the pet was returned")
		}
	}
	if _, ok := h.srv.State.Object(pet.ObjectID()); ok {
		t.Fatal("the returned pet is still in the world")
	}
	// The rest of the return's own answer; the swing is still in flight.
	for _, frame := range readImmediate(h.client) {
		if isOwnerInteractFrame(frame, h.ownerID, pet) {
			t.Fatalf("the queued interact ran against the returned pet: opcode %#x", frame[0])
		}
	}

	released := false
	for end := h.client.Now().Add(3 * time.Second); h.client.Now().Before(end); {
		frame := h.client.ReadWithTimeout(end.Sub(h.client.Now()))
		if frame == nil {
			break
		}
		switch {
		case isOwnerInteractFrame(frame, h.ownerID, pet):
			t.Fatalf("the queued interact ran against the returned pet: opcode %#x", frame[0])
		case frame[0] == serverpackets.OpcodeAttack && wire.NewReader(frame[1:]).ReadInt32() == h.ownerID:
			t.Fatal("the owner swung again after its queued interact's pet was returned")
		case frame[0] == serverpackets.OpcodeActionFailed:
			released = true
		}
	}
	if !released {
		t.Fatal("the swing's end never released the queued interact with ActionFailed")
	}
}

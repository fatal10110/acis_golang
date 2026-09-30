package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestQueuedSitDropsAttackBeforeLaterPetInteract(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	hostile := h.srv.SpawnHostileNPCAt(t, location.Location{X: 60, Y: 20, Z: 30})
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), 60, 20, 30, false))
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(hostile.ObjectID(), 60, 20, 30, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeAttack, "attack before sit")
	h.client.Send(encodeRequestChangeWaitType(false))
	for _, frame := range testsupport.SyncBarrierFrames(t, h.client, func() {
		h.client.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestItemList).Bytes())
	}, serverpackets.OpcodeItemList) {
		if frame[0] == serverpackets.OpcodeChangeWaitType {
			t.Fatal("sit ran before swing finished")
		}
	}
	readUntilOpcode(t, h.client, serverpackets.OpcodeChangeWaitType, "sit after swing")
	h.srv.Advance(t, 2500*time.Millisecond)
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeRequestChangeWaitType(true))
	readUntilOpcode(t, h.client, serverpackets.OpcodeChangeWaitType, "stand")
	h.srv.Advance(t, 2500*time.Millisecond)
	drainUntilQuiet(t, h.client)

	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	placePet(t, pet, location.Location{X: px + 300, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), false))
	frames := drainFrames(t, h.client)
	h.srv.Advance(t, 5*time.Second)
	frames = append(frames, drainFrames(t, h.client)...)
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeMoveToPawn {
			continue
		}
		r := wire.NewReader(frame[1:])
		if mover, target := r.ReadInt32(), r.ReadInt32(); mover == h.ownerID && target == hostile.ObjectID() {
			t.Fatal("old attack chased the hostile after pet interaction")
		}
	}
	if !hasOpcode(frames, serverpackets.OpcodePetStatusShow) {
		t.Fatalf("pet interaction did not finish: %x", frameOpcodes(frames))
	}
	if hasOpcode(frames, serverpackets.OpcodeAttack) {
		t.Fatalf("old attack resumed after pet interaction: %x", frameOpcodes(frames))
	}
}

func TestOwnedPetInteractWaitsForStandUp(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	placePet(t, pet, location.Location{X: px + 50, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeAction(pet.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeRequestChangeWaitType(false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeChangeWaitType, "sit")
	h.srv.Advance(t, 2500*time.Millisecond)
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeRequestChangeWaitType(true))
	readUntilOpcode(t, h.client, serverpackets.OpcodeChangeWaitType, "stand")
	clickPetQueued(t, h, pet, "during stand-up")
	h.srv.Advance(t, 2500*time.Millisecond)
	readUntilOpcode(t, h.client, serverpackets.OpcodePetStatusShow, "pet status after stand-up")
}

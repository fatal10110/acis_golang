package network

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// testSummon is a minimal worldobject.Object double standing in for a
// *summon.Actor: broadcastRelations only ever reads its ObjectID, since a
// summon is never a frameReceiver itself (its owner's client shows its
// relation icon, not a connection of its own).
type testSummon struct{ id int32 }

func (s testSummon) ObjectID() int32 { return s.id }

func relationChangedPayload(objectID, relation, autoAttackable, karma, pvpFlag int32) []byte {
	want := []byte{serverpackets.OpcodeRelationChanged}
	for _, v := range []int32{objectID, relation, autoAttackable, karma, pvpFlag} {
		want = binary.LittleEndian.AppendUint32(want, uint32(v))
	}
	return want
}

func TestBroadcastRelationsWithoutSummonNotifiesObserverOnly(t *testing.T) {
	state := world.New()
	selfFrames := &frameCapture{}
	observerFrames := &frameCapture{}
	self := newTestLivePlayer(t, 1, selfFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)
	self.Character.KarmaPoints = 500
	state.Spawn(self, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	// Spawn's mutual Discover already exchanged CharInfo frames between the
	// two; clear those before exercising broadcastRelations in isolation.
	selfFrames.frames = nil
	observerFrames.frames = nil

	link := &GameClientLink{world: state}
	link.broadcastRelations(self)

	if len(selfFrames.frames) != 0 {
		t.Fatalf("self frames = %d, want 0: broadcastRelationsChanges excludes the subject itself", len(selfFrames.frames))
	}
	if len(observerFrames.frames) != 1 {
		t.Fatalf("observer frames = %d, want 1", len(observerFrames.frames))
	}
	want := relationChangedPayload(self.ObjectID(), serverpackets.RelationHasKarma, 1, 500, 0)
	if !bytes.Equal(observerFrames.frames[0], want) {
		t.Fatalf("observer frame = %x, want %x", observerFrames.frames[0], want)
	}
}

func TestBroadcastRelationsPvPZoneIsPerObserver(t *testing.T) {
	state := world.New()
	selfFrames := &frameCapture{}
	insideFrames := &frameCapture{}
	outsideFrames := &frameCapture{}
	self := newTestLivePlayer(t, 1, selfFrames)
	inside := newTestLivePlayer(t, 2, insideFrames)
	outside := newTestLivePlayer(t, 3, outsideFrames)
	self.SetInPvPZone(true)
	inside.SetInPvPZone(true)
	state.Spawn(self, 0, 0, 0, 0)
	state.Spawn(inside, 100, 0, 0, 0)
	state.Spawn(outside, -100, 0, 0, 0)
	selfFrames.frames = nil
	insideFrames.frames = nil
	outsideFrames.frames = nil

	(&GameClientLink{world: state}).broadcastRelations(self)

	wantInside := relationChangedPayload(self.ObjectID(), 0, 1, 0, 0)
	if len(insideFrames.frames) != 1 || !bytes.Equal(insideFrames.frames[0], wantInside) {
		t.Fatalf("inside observer frames = %x, want %x", insideFrames.frames, wantInside)
	}
	wantOutside := relationChangedPayload(self.ObjectID(), 0, 0, 0, 0)
	if len(outsideFrames.frames) != 1 || !bytes.Equal(outsideFrames.frames[0], wantOutside) {
		t.Fatalf("outside observer frames = %x, want %x", outsideFrames.frames, wantOutside)
	}
}

func TestBroadcastSummonSpawnRelationPvPZoneIsPerObserver(t *testing.T) {
	state := world.New()
	selfFrames := &frameCapture{}
	insideFrames := &frameCapture{}
	outsideFrames := &frameCapture{}
	self := newTestLivePlayer(t, 1, selfFrames)
	inside := newTestLivePlayer(t, 2, insideFrames)
	outside := newTestLivePlayer(t, 3, outsideFrames)
	self.SetInPvPZone(true)
	inside.SetInPvPZone(true)
	state.Spawn(self, 0, 0, 0, 0)
	state.Spawn(inside, 100, 0, 0, 0)
	state.Spawn(outside, -100, 0, 0, 0)
	selfFrames.frames = nil
	insideFrames.frames = nil
	outsideFrames.frames = nil

	(&GameClientLink{world: state}).broadcastSummonSpawnRelation(self, testSummon{id: 77})

	wantInside := relationChangedPayload(77, 0, 1, 0, 0)
	if len(insideFrames.frames) != 1 || !bytes.Equal(insideFrames.frames[0], wantInside) {
		t.Fatalf("inside observer frames = %x, want %x", insideFrames.frames, wantInside)
	}
	wantOutside := relationChangedPayload(77, 0, 0, 0, 0)
	if len(outsideFrames.frames) != 1 || !bytes.Equal(outsideFrames.frames[0], wantOutside) {
		t.Fatalf("outside observer frames = %x, want %x", outsideFrames.frames, wantOutside)
	}
}

func TestBroadcastRelationsWithSummonNotifiesSelfAndBroadcastsBoth(t *testing.T) {
	state := world.New()
	selfFrames := &frameCapture{}
	observerFrames := &frameCapture{}
	self := newTestLivePlayer(t, 1, selfFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)
	self.Character.UpdatePvPFlag(task.PvPFlagOn)
	state.Spawn(self, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	pet := testSummon{id: 77}
	state.AddSummon(self.ObjectID(), pet)
	// Spawn's mutual Discover already exchanged CharInfo frames between the
	// two; clear those before exercising broadcastRelations in isolation.
	selfFrames.frames = nil
	observerFrames.frames = nil

	link := &GameClientLink{world: state}
	link.broadcastRelations(self)

	if len(selfFrames.frames) != 1 {
		t.Fatalf("self frames = %d, want 1 (summon self-view)", len(selfFrames.frames))
	}
	wantSelf := relationChangedPayload(pet.ObjectID(), serverpackets.RelationPvPFlag, 0, 0, 1)
	if !bytes.Equal(selfFrames.frames[0], wantSelf) {
		t.Fatalf("self frame = %x, want %x", selfFrames.frames[0], wantSelf)
	}

	if len(observerFrames.frames) != 2 {
		t.Fatalf("observer frames = %d, want 2 (owner + summon)", len(observerFrames.frames))
	}
	wantOwner := relationChangedPayload(self.ObjectID(), serverpackets.RelationPvPFlag, 1, 0, 1)
	wantPet := relationChangedPayload(pet.ObjectID(), serverpackets.RelationPvPFlag, 1, 0, 1)
	if !bytes.Equal(observerFrames.frames[0], wantOwner) {
		t.Fatalf("observer frame[0] (owner) = %x, want %x", observerFrames.frames[0], wantOwner)
	}
	if !bytes.Equal(observerFrames.frames[1], wantPet) {
		t.Fatalf("observer frame[1] (summon) = %x, want %x", observerFrames.frames[1], wantPet)
	}
}

func TestBroadcastRelationsNoopWithoutWorld(t *testing.T) {
	self := newTestLivePlayer(t, 1, &frameCapture{})
	link := &GameClientLink{}

	// Should not panic when no world is wired.
	link.broadcastRelations(self)
}

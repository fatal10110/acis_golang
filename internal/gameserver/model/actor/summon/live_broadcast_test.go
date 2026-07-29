package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type broadcastFrameReceiver struct {
	world.Presence
	trackedID int32
	frames    [][]byte
}

func (f *broadcastFrameReceiver) ObjectID() int32 { return f.trackedID }

func (f *broadcastFrameReceiver) SendFrame(frame wire.Frame) bool {
	defer frame.Release()
	raw := frame.Bytes()
	payload := make([]byte, len(raw)-2)
	copy(payload, raw[2:])
	f.frames = append(f.frames, payload)
	return true
}

func TestActorBroadcastFrameReachesOwnKnownObservers(t *testing.T) {
	state := world.New()
	owner := &liveOwnerStub{id: 100, level: 40}
	state.Spawn(owner, 1000, 2000, -50, 32768)
	state.AddPlayer(owner)

	actor := NewServitor(ServitorConfig{ObjectID: 200, Owner: owner, Level: 44})
	SpawnBesideOwner(state, actor, owner, location.Location{})

	observer := &broadcastFrameReceiver{trackedID: 300}
	state.Spawn(observer, 1000, 2000, -50, 0)

	substitute := &broadcastFrameReceiver{trackedID: 400}
	state.Spawn(substitute, 100000, 100000, -50, 0)

	self := serverpackets.SkillCastObject{ObjectID: actor.ObjectID()}
	actor.BroadcastFrame(serverpackets.FrameMagicSkillUse(self, self, 1, 1, 0, 0, false))

	if len(observer.frames) != 1 {
		t.Fatalf("observer received %d frames, want 1", len(observer.frames))
	}
	if observer.frames[0][0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("frame opcode = %#x, want %#x", observer.frames[0][0], serverpackets.OpcodeMagicSkillUse)
	}
	if len(substitute.frames) != 0 {
		t.Fatalf("out-of-range substitute received %d frames, want 0", len(substitute.frames))
	}
}

func TestActorBroadcastFrameNoopsWithoutWorld(t *testing.T) {
	actor := NewServitor(ServitorConfig{ObjectID: 200, Level: 44})
	self := serverpackets.SkillCastObject{ObjectID: actor.ObjectID()}
	actor.BroadcastFrame(serverpackets.FrameMagicSkillUse(self, self, 1, 1, 0, 0, false))
}

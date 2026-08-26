package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// fakeSummonOwner is a minimal owner for SpawnBesideOwner fixtures.
type fakeSummonOwner struct {
	world.Presence

	id int32
}

func (o *fakeSummonOwner) ObjectID() int32           { return o.id }
func (o *fakeSummonOwner) LevelValue() int           { return 1 }
func (o *fakeSummonOwner) Position() (int, int, int) { return 1000, 1000, 0 }
func (o *fakeSummonOwner) InCombat() bool            { return false }

// fakeBroadcastReceiver is a world-visible observer that retains every
// frame handed to it, so a test can assert delivery and ownership.
type fakeBroadcastReceiver struct {
	world.Presence

	id     int32
	frames []wire.Frame
}

func (r *fakeBroadcastReceiver) ObjectID() int32 { return r.id }
func (r *fakeBroadcastReceiver) BroadcastFrame(frame wire.Frame) bool {
	r.frames = append(r.frames, frame)
	return true
}

// fakeFrameBuilder counts every translation call the domain makes and answers
// with a throwaway frame long enough to survive wire.CopyFrame.
type fakeFrameBuilder struct {
	FrameBuilder

	attacks     int
	moves       int
	stops       int
	moveToPawns int
	skillUses   int
}

func frameBytes(n int) []byte { return make([]byte, n) }

func (f *fakeFrameBuilder) Attack(snapshot attack.Snapshot) wire.Frame {
	f.attacks++
	return wire.BorrowedFrame(frameBytes(8))
}

func (f *fakeFrameBuilder) Move(objectID int32, event move.Event) wire.Frame {
	f.moves++
	return wire.BorrowedFrame(frameBytes(8))
}

func (f *fakeFrameBuilder) MoveToPawn(objectID, targetID int32, distance int, origin location.Location) wire.Frame {
	f.moveToPawns++
	return wire.BorrowedFrame(frameBytes(8))
}

func (f *fakeFrameBuilder) Stop(objectID int32, at location.Location, heading int) wire.Frame {
	f.stops++
	return wire.BorrowedFrame(frameBytes(8))
}

func (f *fakeFrameBuilder) SkillUse(casterID int32, casterAt location.Location, targetID int32, targetAt location.Location, skillID, level int32, hitTime, reuseDelay int, success bool) wire.Frame {
	f.skillUses++
	return wire.BorrowedFrame(frameBytes(8))
}

type broadcastFixture struct {
	actor    *Actor
	receiver *fakeBroadcastReceiver
	other    *fakeBroadcastReceiver
	frames   *fakeFrameBuilder
}

func newBroadcastFixture(t *testing.T) broadcastFixture {
	t.Helper()
	state := world.New()
	actor := NewServitor(ServitorConfig{ObjectID: 7})
	owner := &fakeSummonOwner{id: 1}
	SpawnBesideOwner(state, actor, owner, location.Location{})
	frames := &fakeFrameBuilder{}
	actor.SetFrameBuilder(frames)
	receiver := &fakeBroadcastReceiver{id: 2}
	state.Spawn(receiver, 1010, 1000, 0, 0)
	other := &fakeBroadcastReceiver{id: 3}
	state.Spawn(other, 1030, 1000, 0, 0)
	return broadcastFixture{actor: actor, receiver: receiver, other: other, frames: frames}
}

func TestSummonBroadcastDeliversTypedFramesThroughHook(t *testing.T) {
	fx := newBroadcastFixture(t)

	if err := fx.actor.BroadcastMove(move.Event{Origin: location.Location{X: 1}, Destination: location.Location{X: 2}}); err != nil {
		t.Fatalf("BroadcastMove() error = %v", err)
	}
	if err := fx.actor.BroadcastStop(); err != nil {
		t.Fatalf("BroadcastStop() error = %v", err)
	}
	if err := fx.actor.BroadcastSelfSkillUse(1422, 1); err != nil {
		t.Fatalf("BroadcastSelfSkillUse() error = %v", err)
	}

	if fx.frames.moves != 1 || fx.frames.stops != 1 || fx.frames.skillUses != 1 {
		t.Fatalf("builder calls moves=%d stops=%d skillUses=%d, want 1/1/1", fx.frames.moves, fx.frames.stops, fx.frames.skillUses)
	}
	if got := len(fx.receiver.frames); got != 3 {
		t.Fatalf("receiver got %d frames, want 3", got)
	}
	for i, frame := range fx.receiver.frames {
		frame.Release()
		_ = i
	}
}

// TestSummonBroadcastHandsEachReceiverAnOwnedCopy proves the fan-out never
// shares one frame between recipients: mutating the first receiver's copy
// must leave every other receiver's bytes unchanged.
func TestSummonBroadcastHandsEachReceiverAnOwnedCopy(t *testing.T) {
	fx := newBroadcastFixture(t)

	if err := fx.actor.BroadcastSelfSkillUse(1422, 1); err != nil {
		t.Fatalf("BroadcastSelfSkillUse() error = %v", err)
	}
	if len(fx.receiver.frames) != 1 || len(fx.other.frames) != 1 {
		t.Fatalf("receiver frames = %d/%d, want 1/1", len(fx.receiver.frames), len(fx.other.frames))
	}
	before := append([]byte(nil), fx.other.frames[0].Bytes()...)
	fx.receiver.frames[0].Bytes()[2] = 0xFF
	if got := fx.other.frames[0].Bytes(); string(got) != string(before) {
		t.Fatalf("mutating one recipient's frame changed another's: %x != %x", got, before)
	}
}

func TestSummonBroadcastAttackDeliversThroughHook(t *testing.T) {
	fx := newBroadcastFixture(t)

	if err := fx.actor.BroadcastAttack(attack.Snapshot{}); err != nil {
		t.Fatalf("BroadcastAttack() error = %v", err)
	}
	if fx.frames.attacks != 1 {
		t.Fatalf("builder attack calls = %d, want 1", fx.frames.attacks)
	}
	if len(fx.receiver.frames) != 1 {
		t.Fatalf("receiver frames = %d, want 1", len(fx.receiver.frames))
	}
}

func TestSummonBroadcastWithoutHookIsSilentNoOp(t *testing.T) {
	state := world.New()
	actor := NewServitor(ServitorConfig{ObjectID: 7})
	SpawnBesideOwner(state, actor, &fakeSummonOwner{id: 1}, location.Location{})

	if err := actor.BroadcastStop(); err != nil {
		t.Fatalf("BroadcastStop() with no builder error = %v, want nil", err)
	}
	if err := actor.BroadcastSelfSkillUse(1422, 1); err != nil {
		t.Fatalf("BroadcastSelfSkillUse() with no builder error = %v, want nil", err)
	}
}

func TestSummonBroadcastMoveToPawnDeliversThroughHook(t *testing.T) {
	fx := newBroadcastFixture(t)

	target := NewServitor(ServitorConfig{ObjectID: 9})
	fx.actor.world.Spawn(target, 1020, 1000, 0, 0)

	if err := fx.actor.BroadcastMoveToPawn(target); err != nil {
		t.Fatalf("BroadcastMoveToPawn() error = %v", err)
	}
	if fx.frames.moveToPawns != 1 {
		t.Fatalf("builder moveToPawn calls = %d, want 1", fx.frames.moveToPawns)
	}
	if len(fx.receiver.frames) != 1 {
		t.Fatalf("receiver frames = %d, want 1", len(fx.receiver.frames))
	}
}

package npc

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestNewEffectPointRejectsUnsupportedTemplate(t *testing.T) {
	if _, err := NewEffectPoint(1, &Template{ID: 1, Type: "Bogus"}, 0); err == nil {
		t.Fatal("NewEffectPoint with an unsupported template type = nil error, want one")
	}
}

func TestEffectPointNeverDeadAndOwnsEffectList(t *testing.T) {
	ep, err := NewEffectPoint(1, &Template{ID: 13018, Type: "EffectPoint"}, 42)
	if err != nil {
		t.Fatalf("NewEffectPoint: %v", err)
	}
	if ep.Dead() {
		t.Fatal("Dead() = true, want false")
	}
	if ep.OwnerID() != 42 {
		t.Fatalf("OwnerID() = %d, want 42", ep.OwnerID())
	}
	if ep.EffectList() == nil {
		t.Fatal("EffectList() = nil")
	}
}

func TestEffectPointSpawnDespawnRegistersInWorld(t *testing.T) {
	state := world.New()
	ep, err := NewEffectPoint(1, &Template{ID: 13018, Type: "EffectPoint"}, 0)
	if err != nil {
		t.Fatalf("NewEffectPoint: %v", err)
	}
	ep.SetWorld(state)
	ep.Spawn(100, 100, 0, 0)

	if _, ok := state.Object(ep.ObjectID()); !ok {
		t.Fatal("actor not tracked in world after Spawn")
	}

	ep.Despawn()
	if _, ok := state.Object(ep.ObjectID()); ok {
		t.Fatal("actor still tracked in world after Despawn")
	}
}

func TestEffectPointSpawnDespawnNoopWithoutWorld(t *testing.T) {
	ep, err := NewEffectPoint(1, &Template{ID: 13018, Type: "EffectPoint"}, 0)
	if err != nil {
		t.Fatalf("NewEffectPoint: %v", err)
	}
	ep.Spawn(100, 100, 0, 0) // must not panic
	ep.Despawn()             // must not panic
}

func TestEffectPointForEachNearbyExcludesSelfAndFindsOthers(t *testing.T) {
	state := world.New()
	ep, err := NewEffectPoint(1, &Template{ID: 13018, Type: "EffectPoint"}, 0)
	if err != nil {
		t.Fatalf("NewEffectPoint: %v", err)
	}
	ep.SetWorld(state)
	ep.Spawn(100, 100, 0, 0)

	inRange := &frameReceiver{trackedID: 55}
	state.Spawn(inRange, 150, 100, 0, 0)

	outOfRange := &frameReceiver{trackedID: 66}
	state.Spawn(outOfRange, 1000, 100, 0, 0)

	var found []int32
	ep.ForEachNearby(200, func(o world.Tracked) {
		found = append(found, o.ObjectID())
	})

	if len(found) != 1 || found[0] != 55 {
		t.Fatalf("ForEachNearby found %v, want [55]", found)
	}
}

func TestEffectPointBroadcastSkillUseAndLaunched(t *testing.T) {
	state := world.New()
	ep, err := NewEffectPoint(1, &Template{ID: 13018, Type: "EffectPoint"}, 0)
	if err != nil {
		t.Fatalf("NewEffectPoint: %v", err)
	}
	ep.SetWorld(state)
	ep.SetFrameBuilder(serverpackets.NpcFrameBuilder{})
	ep.Spawn(100, 100, 0, 0)

	observer := &frameReceiver{trackedID: 55}
	state.Spawn(observer, 100, 100, 0, 0)

	target := &frameReceiver{trackedID: 77}
	state.Spawn(target, 150, 100, 0, 0)

	ep.BroadcastSkillUse(target, 454, 1)
	ep.BroadcastSkillLaunched(454, 1, []int32{77})

	if len(observer.frames) != 2 {
		t.Fatalf("observer received %d frames, want 2", len(observer.frames))
	}

	wantUse := serverpackets.FrameMagicSkillUse(
		serverpackets.SkillCastObject{ObjectID: ep.ObjectID(), Location: location.Location{X: 100, Y: 100, Z: 0}},
		serverpackets.SkillCastObject{ObjectID: target.ObjectID(), Location: location.Location{X: 150, Y: 100, Z: 0}},
		454, 1, 0, 0, false,
	)
	if !bytes.Equal(observer.frames[0], wantUse.Bytes()[2:]) {
		t.Fatalf("frame[0] = %x, want %x", observer.frames[0], wantUse.Bytes()[2:])
	}

	wantLaunched := serverpackets.FrameMagicSkillLaunched(ep.ObjectID(), 454, 1, []int32{77})
	if !bytes.Equal(observer.frames[1], wantLaunched.Bytes()[2:]) {
		t.Fatalf("frame[1] = %x, want %x", observer.frames[1], wantLaunched.Bytes()[2:])
	}
}

func TestEffectPointBroadcastNoopWithoutWorld(t *testing.T) {
	ep, err := NewEffectPoint(1, &Template{ID: 13018, Type: "EffectPoint"}, 0)
	if err != nil {
		t.Fatalf("NewEffectPoint: %v", err)
	}
	ep.BroadcastSkillUse(&frameReceiver{trackedID: 2}, 454, 1) // must not panic
	ep.BroadcastSkillLaunched(454, 1, nil)                     // must not panic
}

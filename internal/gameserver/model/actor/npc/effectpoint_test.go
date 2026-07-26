package npc

import (
	"testing"

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

	nearby := &frameReceiver{trackedID: 55}
	state.Spawn(nearby, 150, 100, 0, 0)

	var found []int32
	ep.ForEachNearby(-1, func(o world.Tracked) {
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
	if observer.frames[0][0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("frame[0] opcode = %#x, want %#x", observer.frames[0][0], serverpackets.OpcodeMagicSkillUse)
	}
	if observer.frames[1][0] != serverpackets.OpcodeMagicSkillLaunched {
		t.Fatalf("frame[1] opcode = %#x, want %#x", observer.frames[1][0], serverpackets.OpcodeMagicSkillLaunched)
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

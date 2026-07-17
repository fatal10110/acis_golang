package world

import "testing"

func TestState_ObjectLifecycle(t *testing.T) {
	s := New()

	obj := stubObject{id: 42}
	s.AddObject(obj)

	got, ok := s.Object(42)
	if !ok || got != obj {
		t.Fatalf("Object(42) = %+v, %v, want %+v, true", got, ok, obj)
	}
	if len(s.Objects()) != 1 {
		t.Fatalf("Objects() = %v, want 1 entry", s.Objects())
	}

	s.RemoveObject(obj)
	if _, ok := s.Object(42); ok {
		t.Fatal("Object(42) present after RemoveObject")
	}
}

func TestState_RemoveObjectIgnoresStaleIdentity(t *testing.T) {
	s := New()

	stale := stubObject{id: 42, gen: 1}
	s.AddObject(stale)
	s.RemoveObject(stale)

	fresh := stubObject{id: 42, gen: 2}
	s.AddObject(fresh) // id 42 now points at fresh, e.g. a respawn that reused the id

	// A despawn racing that respawn must not evict fresh.
	s.RemoveObject(stale)

	got, ok := s.Object(42)
	if !ok || got != fresh {
		t.Fatalf("Object(42) = %+v, %v, want the fresh occupant %+v, true", got, ok, fresh)
	}
}

func TestState_RemoveObjects(t *testing.T) {
	s := New()
	s.AddObject(stubObject{id: 1})
	s.AddObject(stubObject{id: 2})
	s.AddObject(stubObject{id: 3})

	s.RemoveObjects([]int32{1, 2})
	if len(s.Objects()) != 1 {
		t.Fatalf("Objects() = %v, want 1 entry after RemoveObjects", s.Objects())
	}
	if _, ok := s.Object(3); !ok {
		t.Fatal("Object(3) missing after removing unrelated ids")
	}
}

func TestState_PlayerLifecycle(t *testing.T) {
	s := New()

	p := stubObject{id: 7}
	s.AddPlayer(p)

	if _, ok := s.Player(7); !ok {
		t.Fatal("Player(7) missing after AddPlayer")
	}
	if len(s.Players()) != 1 {
		t.Fatalf("Players() = %v, want 1 entry", s.Players())
	}

	s.RemovePlayer(7)
	if _, ok := s.Player(7); ok {
		t.Fatal("Player(7) present after RemovePlayer")
	}
}

func TestState_PetKeyedByOwner(t *testing.T) {
	s := New()

	pet := stubObject{id: 99} // the pet's own id differs from its owner's
	const ownerID = 7
	s.AddPet(ownerID, pet)

	got, ok := s.Pet(ownerID)
	if !ok || got != pet {
		t.Fatalf("Pet(%d) = %+v, %v, want %+v, true", ownerID, got, ok, pet)
	}
	if _, ok := s.Pet(pet.id); ok {
		t.Fatal("Pet lookup must use the owner id, not the pet's own id")
	}

	s.RemovePet(ownerID)
	if _, ok := s.Pet(ownerID); ok {
		t.Fatal("Pet present after RemovePet")
	}
}

func TestState_EmbedsGrid(t *testing.T) {
	s := New()

	r, ok := s.RegionAt(MinX, MinY)
	if !ok {
		t.Fatal("RegionAt via embedded Grid failed for a valid coordinate")
	}
	if len(s.Neighbors(r, 0)) != 1 {
		t.Error("Neighbors via embedded Grid did not return the region itself")
	}
}

package world

import "testing"

// TestRemovePlayerLeavesAnotherPlayerWithTheSameID pins RemovePlayer's
// identity check: removing a player that is not the one tracked under its id
// leaves the tracked one online and findable by name.
func TestRemovePlayerLeavesAnotherPlayerWithTheSameID(t *testing.T) {
	s := New()
	winner := &namedPlayerObject{id: 1, name: "Newbie"}
	loser := &namedPlayerObject{id: 1, name: "Newbie"}
	s.AddPlayer(winner)
	s.RemovePlayer(loser)
	if got, ok := s.Player(1); !ok || got != winner {
		t.Fatalf("Player(1) = %v, %v after another player's removal, want the tracked one", got, ok)
	}
	if got, ok := s.PlayerByName("Newbie"); !ok || got != winner {
		t.Fatalf("PlayerByName = %v, %v after another player's removal, want the tracked one", got, ok)
	}
	s.RemovePlayer(winner)
	if _, ok := s.Player(1); ok {
		t.Fatal("tracked player still online after its own removal")
	}
	if _, ok := s.PlayerByName("Newbie"); ok {
		t.Fatal("tracked player still indexed by name after its own removal")
	}
}

package player

import "testing"

func TestCharacterIsPlayer(t *testing.T) {
	if _, ok := any(&Character{}).(interface{ IsPlayer() bool }); !ok {
		t.Fatal("Character does not identify itself as a player to effects")
	}
}

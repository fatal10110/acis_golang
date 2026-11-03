package serverpackets

import "testing"

// TestSystemMessageLooting pins the loot rule messages: 1031 finders keepers
// to 1035 by turn including spoil, in the client's loot rule order.
func TestSystemMessageLooting(t *testing.T) {
	for rule, want := range []int{1031, 1032, 1033, 1034, 1035} {
		if got := SystemMessageLooting(int32(rule)); got != want {
			t.Errorf("SystemMessageLooting(%d) = %d, want %d", rule, got, want)
		}
	}
}

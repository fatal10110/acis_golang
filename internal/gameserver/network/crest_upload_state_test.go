package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

// RequestSetPledgeCrest (0x53) is an in-game-only packet: a client at the
// login handshake, at character select or still entering the world has it
// refused.
func TestAllowedGatesRequestSetPledgeCrest(t *testing.T) {
	const op byte = clientpackets.OpcodeRequestSetPledgeCrest
	if !Allowed(StateInGame, op) {
		t.Fatal("Allowed(in-game, RequestSetPledgeCrest) = false, want true")
	}
	for _, s := range []State{StateConnected, StateAuthed, StateEntering} {
		if Allowed(s, op) {
			t.Fatalf("Allowed(%s, RequestSetPledgeCrest) = true, want false", s)
		}
	}
}

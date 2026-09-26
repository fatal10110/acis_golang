package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestPetStrikeReportsItsDamageToTheOwner lands a pet's PDAM strike on the
// fixture monster: the owner reads the pet damage message carrying the HP
// the monster lost (at least all of it, when the strike kills), never the
// player's own damage message.
func TestPetStrikeReportsItsDamageToTheOwner(t *testing.T) {
	t.Parallel()
	strike := wolfStrike()
	strike.Power = 1
	h, _, hostile := bootWolfStrikerWith(t, strike)
	full := float64(hostile.MaxHP())
	startWolfStrike(t, h)
	h.srv.AdvanceUntil(t, "strike landing on the monster", func() bool { return hostile.HP() < full })
	lost, killed := int32(full-hostile.HP()), hostile.Dead()

	found := false
	for _, frame := range drainFrames(t, h.client) {
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(frame[1:])
		switch id := r.ReadInt32(); id {
		case serverpackets.SystemMessagePetHitForS1Damage:
			params, typ, amount := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
			if params != 1 || typ != serverpackets.SystemMessageParamNumber || amount < lost || !killed && amount != lost {
				t.Fatalf("pet damage message = params %d type %d amount %d, want one number %d", params, typ, amount, lost)
			}
			found = true
		case serverpackets.SystemMessageYouDidS1Dmg, serverpackets.SystemMessageSummonGaveDamageS1:
			t.Fatalf("owner read damage message %d, want the pet family", id)
		}
	}
	if !found {
		t.Fatal("pet damage message never reached the owner")
	}
}

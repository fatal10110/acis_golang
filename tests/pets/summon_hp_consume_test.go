package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestInvulPetPaysStrikeHPCostAndTellsOwner pins that a pet's own skill HP
// cost is consumption, not a hit: invulnerability does not waive it, and
// the owner is told of the damage with the pet itself named as its source.
func TestInvulPetPaysStrikeHPCostAndTellsOwner(t *testing.T) {
	const hpCost = 7
	strike := wolfStrike()
	strike.HPConsume = hpCost
	h, petActor, _ := bootWolfStrikerWith(t, strike)
	petActor.SetInvul(true)
	before := petActor.HP()

	startWolfStrike(t, h)
	h.srv.AdvanceUntil(t, "the pet paying its strike's HP cost", func() bool { return petActor.HP() == before-hpCost })

	frame := findSystemMessage(t, drainFrames(t, h.client), serverpackets.SystemMessagePetReceivedS2DamageByS1)
	if frame == nil {
		t.Fatal("owner was not told of the damage the pet's HP cost dealt it")
	}
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // message id
	if n := r.ReadInt32(); n != 2 {
		t.Fatalf("damage message params = %d, want 2", n)
	}
	r.ReadInt32()
	if name, want := r.ReadString(), petActor.CharacterName(); name != want {
		t.Fatalf("damage source = %q, want the pet itself %q", name, want)
	}
	r.ReadInt32()
	if dmg := r.ReadInt32(); dmg != hpCost {
		t.Fatalf("damage amount = %d, want the HP cost %d", dmg, hpCost)
	}
}

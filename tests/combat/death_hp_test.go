package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func TestDirectDeathClearsHP(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	startInWorld(t, srv.Client)
	obj, ok := srv.State.Player(srv.SoleObjectID(t))
	if !ok {
		t.Fatal("player missing from world")
	}
	player := obj.(interface {
		Die(attackable.Combatant) bool
		CurrentHP() int
		SetHP(float64)
		ReduceCurrentHP(int) bool
	})
	if player.CurrentHP() <= 0 || !player.Die(nil) {
		t.Fatal("direct death did not kill a living player")
	}
	if got := player.CurrentHP(); got != 0 {
		t.Fatalf("dead player HP = %d, want 0", got)
	}
	player.SetHP(10) // Simulate a stale positive HP value while dead.
	if player.ReduceCurrentHP(1) || player.CurrentHP() != 10 {
		t.Fatal("dead player paid an HP cost")
	}

	npc := srv.SpawnHostileNPC(t)
	if npc.CurrentHP() <= 0 || !npc.Die(nil, nil) {
		t.Fatal("direct death did not kill a living NPC")
	}
	if got := npc.CurrentHP(); got != 0 {
		t.Fatalf("dead NPC HP = %d, want 0", got)
	}
}

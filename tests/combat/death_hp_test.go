package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func TestDirectDeathClearsHP(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	startInWorld(t, srv.Client)
	attackerRow := srv.SeedCharacterFor(t, "attacker", "Attacker", 5, 0)
	attackerClient := srv.DialClient(t, "attacker", 1)
	startInWorld(t, attackerClient)
	attackerObj, ok := srv.State.Player(attackerRow.ID)
	if !ok {
		t.Fatal("attacker missing from world")
	}
	attacker := attackerObj.(attackable.Combatant)
	dotAttacker := attackerObj.(effect.Actor)
	obj, ok := srv.State.Player(srv.SoleObjectID(t))
	if !ok {
		t.Fatal("player missing from world")
	}
	player := obj.(interface {
		Die(attackable.Combatant) bool
		CurrentHP() int
		CurrentCP() int
		SetHP(float64)
		SetCP(float64)
		SetSpawnProtection(bool)
		ReduceCurrentHP(int) bool
		ReduceHP(float64, attackable.Combatant, modelskill.Definition)
		ReduceHPByDOT(float64, effect.Actor, bool)
	})
	player.SetSpawnProtection(false)
	player.SetCP(10)
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
	beforeCP := player.CurrentCP()
	if beforeCP <= 0 {
		t.Fatal("fixture player has no CP to protect")
	}
	player.ReduceHP(1, attacker, modelskill.Definition{})
	if hp, cp := player.CurrentHP(), player.CurrentCP(); hp != 10 || cp != beforeCP {
		t.Fatalf("skill damage changed dead player's HP/CP to %d/%d, want 10/%d", hp, cp, beforeCP)
	}
	player.ReduceHPByDOT(1, dotAttacker, true)
	if hp, cp := player.CurrentHP(), player.CurrentCP(); hp != 10 || cp != beforeCP {
		t.Fatalf("DOT changed dead player's HP/CP to %d/%d, want 10/%d", hp, cp, beforeCP)
	}

	npc := srv.SpawnHostileNPC(t)
	if npc.CurrentHP() <= 0 || !npc.Die(nil, nil) {
		t.Fatal("direct death did not kill a living NPC")
	}
	if got := npc.CurrentHP(); got != 0 {
		t.Fatalf("dead NPC HP = %d, want 0", got)
	}
}

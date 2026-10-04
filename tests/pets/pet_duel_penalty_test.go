package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPetDeathPenaltySkippedWhileOwnerDuels: Pet.doDie (Pet.java:261-263)
// takes no experience from a pet whose owner isInDuel(). The wolf would
// lose 29 of its 700 experience outside the duel (TestPetDeathPenalty).
func TestPetDeathPenaltySkippedWhileOwnerDuels(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{penaltyWolfTemplate(), treeTemplate()})),
	})
	if err := h.srv.Pets.Save(petCtx(), h.collarID, pet.State{
		Level: wolfLevel, Exp: 700, CurHP: wolfMaxHP, CurMP: wolfMaxMP, Fed: wolfMaxMeal,
	}); err != nil {
		t.Fatalf("seed pets row: %v", err)
	}
	wolf, _ := h.spawnWolf(t)
	h.srv.SeedCharacterFor(t, "player2", "Rival", 20, 0)
	rival := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, rival)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, rival)

	obj, _ := h.srv.State.Player(h.ownerID)
	owner := obj.(interface {
		attackable.Combatant
		InDuel() bool
		DuelState() duel.State
	})
	h.client.Send(encodePetDuelChallenge("Rival"))
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, rival)
	rival.Send(encodePetDuelAccept())
	h.srv.AdvanceUntil(t, "the duel starting", func() bool { return owner.DuelState() == duel.Duelling })
	drainUntilQuiet(t, h.client)

	wolf.ReduceHP(wolf.HP()+1, owner, modelskill.Definition{})
	h.srv.AdvanceUntil(t, "wolf dead", wolf.Dead)
	h.srv.Settle(t)
	if !owner.InDuel() {
		t.Fatal("the owner left the duel before its pet died; the gate was not exercised")
	}
	if got := wolf.Exp(); got != 700 {
		t.Fatalf("wolf killed while its owner duels: exp = %d, want 700 kept", got)
	}
	if got := wolf.Level(); got != wolfLevel {
		t.Fatalf("wolf killed while its owner duels: level = %d, want %d kept", got, wolfLevel)
	}
}

func encodePetDuelChallenge(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestDuelStart)
	w.WriteString(name)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodePetDuelAccept() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestDuelAnswerStart)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(1)
	return w.Bytes()
}

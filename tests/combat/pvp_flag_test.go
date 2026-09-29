package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestForcedAttackOnAPlayerFlagsTheAttacker has a player force-attack an
// unflagged, karma-free player. The attack flags the attacker, and the
// victim is told through the attacker's RelationChanged, which now carries
// the PvP flag.
func TestForcedAttackOnAPlayerFlagsTheAttacker(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Attacker", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithPvPFlags(task.NewPvPFlags(task.DefaultPvPFlagOptions(), nil)),
	)
	c, attackerID := srv.Client, srv.SoleObjectID(t)
	// A level 40 victim survives the hit: killing an innocent player would
	// make the attacker a PKer, which ends its PvP flag again.
	victim := srv.SeedCharacterFor(t, "victim", "Victim", 40, 0)
	vc := srv.DialClient(t, "victim", 1)
	startInWorld(t, vc)
	startInWorld(t, c)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)

	obj, ok := srv.State.Player(attackerID)
	if !ok {
		t.Fatal("attacker missing from world state")
	}
	// Every swing hits, so the scenario never waits on a lucky roll.
	done := make(chan struct{})
	if !srv.PlayerQueue(t, attackerID).Post(func() {
		defer close(done)
		obj.(interface{ SetRollSource(func(int) int) }).SetRollSource(func(int) int { return 0 })
	}) {
		t.Fatal("attacker queue closed")
	}
	<-done

	selectPlayerTarget(t, c, victim.ID)
	c.Send(encodeAttackRequest(victim.ID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	attacker := obj.(interface{ PvPFlagState() task.PvPFlagState })
	srv.AdvanceUntil(t, "attacker PvP-flagged by its attack", func() bool {
		return attacker.PvPFlagState() != task.PvPFlagNone
	})

	srv.Settle(t)
	for _, frame := range readQuiet(vc) {
		if frame[0] != serverpackets.OpcodeRelationChanged {
			continue
		}
		r := wireReader(frame[1:])
		if r.ReadInt32() != attackerID {
			continue
		}
		relation, _, _ := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		if pvpFlag := r.ReadInt32(); relation&serverpackets.RelationPvPFlag == 0 || pvpFlag == 0 {
			t.Fatalf("victim's RelationChanged for the attacker = relation %#x flag %d, want the PvP flag", relation, pvpFlag)
		}
		return
	}
	t.Fatal("victim never received the attacker's RelationChanged")
}

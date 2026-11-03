package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPlayerHitRecordsRegisteredPlayerOnNPC pins one identity for a player
// attacker: the monster a player's swing lands on records, as its attacker,
// the very object the world registers the player under, so any identity
// comparison or type check on that attacker sees the registered player.
func TestPlayerHitRecordsRegisteredPlayerOnNPC(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 30, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	registered := obj.(attackable.Combatant)
	onQueue(t, srv.PlayerQueue(t, objID), func() {
		obj.(interface{ SetRollSource(func(int) int) }).SetRollSource(landNoCrit())
	})

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	_, _ = firstHitFeedback(t, c, objID)
	// The hit's feedback reaches the client from inside the player's hit
	// task, before that task applies the damage and records the threat, so
	// wait for the task to finish before reading the monster's table.
	onQueue(t, srv.PlayerQueue(t, objID), func() {})

	var threat attackable.Threat
	onHostileQueue(t, hostile, func() {
		threat, ok = hostile.AI().Threats().Get(registered)
	})
	if !ok {
		t.Fatal("monster has no threat entry for the player whose hit landed")
	}
	if threat.Attacker != registered {
		t.Fatalf("monster's recorded attacker = %T %p, want the registered player %T %p", threat.Attacker, threat.Attacker, registered, registered)
	}
}

// TestPlayerKillOfKarmaPlayerRaisesNoDeathPenalty pins that a player killed
// by another player's swing gains no death-penalty level, even while
// carrying karma: the killer is a player.
func TestPlayerKillOfKarmaPlayerRaisesNoDeathPenalty(t *testing.T) {
	t.Parallel()
	srv, c, vc, _, victim := bootPvPPair(t)
	victimID := victim.ObjectID()
	onQueue(t, srv.PlayerQueue(t, victimID), func() {
		victim.SetCP(0)
		victim.SetHP(1)
	})

	attackPlayer(t, c, victimID)
	srv.AdvanceUntil(t, "victim killed", func() bool { return srv.PlayerDead(t, victimID) })
	srv.Settle(t)

	frames := readQuiet(vc)
	if indexOf(frames, 0, serverpackets.OpcodeDie, victimID) < 0 {
		t.Fatal("victim never received its own Die")
	}
	if i := indexOfSystemMessage(frames, 0, serverpackets.SystemMessageDeathPenaltyLevelS1Added); i >= 0 {
		t.Fatalf("death-penalty message at frame %d, want none for a player kill", i)
	}
	if got := victim.(interface{ DeathPenaltyLevel() int }).DeathPenaltyLevel(); got != 0 {
		t.Fatalf("victim death-penalty level = %d, want 0 after a player kill", got)
	}
}

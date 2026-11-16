package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// swingTally counts what a player read of its own swing: its damage
// feedback (YOU_DID_S1_DMG), critical-hit feedback, ActionFailed answers,
// and the new swings it started.
type swingTally struct {
	damage, crits, actionFailed, swings int
}

// tallySwing reads what the server has queued to c without letting time
// pass and counts objID's swing traffic in it.
func tallySwing(t *testing.T, srv *gameservertest.Server, c *scriptedClient, objID int32) swingTally {
	t.Helper()
	var n swingTally
	for _, frame := range srv.ReadQueued(t, c) {
		switch {
		case attackFrameBy(frame, objID):
			n.swings++
		case frame[0] == serverpackets.OpcodeActionFailed:
			n.actionFailed++
		case frame[0] == serverpackets.OpcodeSystemMessage:
			switch wireReader(frame[1:]).ReadInt32() {
			case serverpackets.SystemMessageYouDidS1Dmg:
				n.damage++
			case serverpackets.SystemMessageCriticalHit:
				n.crits++
			}
		}
	}
	return n
}

// playerAttackMillis is objID's attack time in milliseconds.
func playerAttackMillis(t *testing.T, srv *gameservertest.Server, objID int32) int {
	t.Helper()
	var millis int
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		millis = formulas.TimeBetweenAttacks(pc.AttackSpeed())
	})
	return millis
}

// TestSwingAbortedAtItsHitIdlesThePlayer pins the hit timer of a swing
// whose main target a third party killed in flight (CreatureAttack.onHitTimer
// stop(), PlayerAttack.stop: PlayableAttack.stop's tryToIdle, then
// clientActionFailed): nothing lands, the player reads exactly one
// ActionFailed at the hit, and its intention is idle, so the emote it asks
// for next is broadcast.
func TestSwingAbortedAtItsHitIdlesThePlayer(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a swing open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	hitAt := time.Duration(playerAttackMillis(t, srv, objID)/2) * time.Millisecond

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	// The read stops at the Attack, so the clock stands at the swing's start.
	readUntilOwnAttack(t, c, objID, "the swing")
	if !hostile.Die(nil, nil) {
		t.Fatal("hostile.Die() = false, want the third-party kill")
	}

	srv.Advance(t, hitAt-time.Millisecond)
	if got := tallySwing(t, srv, c, objID); got != (swingTally{}) {
		t.Fatalf("before the hit: %+v, want nothing", got)
	}
	srv.Advance(t, time.Millisecond)
	if got, want := tallySwing(t, srv, c, objID), (swingTally{actionFailed: 1}); got != want {
		t.Fatalf("at the hit: %+v, want %+v", got, want)
	}

	c.Send(encodeRequestSocialAction(2))
	assertEmoteShown(t, c, objID, 2, "emote after the aborted swing")
}

package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference for the tests below (#4971): a ground-item click goes through
// PlayableAI.tryToPickUp, whose first check refuses with ActionFailed alone
// while denyAiAction() holds (PlayableAI.java:411-417), before the
// attack/cast/sit deferral. Teleporting and ImmobileUntilAttacked are the
// denying states the crowd-control gate shared with UseItem does not cover
// (Creature.java:636-639, Player.java:627-630).

// bootDenyAIPicker boots a player with a sword lying at its feet.
func bootDenyAIPicker(t *testing.T) (*gameservertest.Server, int32, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	startInWorld(t, srv.Client)
	srv.SeedGroundItem(t, objID, 30, 1, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, srv.Client)
	return srv, objID, soleGroundObjectID(t, srv)
}

// assertDeniedPickup clicks the ground item and requires ActionFailed alone,
// with the item left on the ground.
func assertDeniedPickup(t *testing.T, srv *gameservertest.Server, groundID int32, what string) {
	t.Helper()
	c := srv.Client
	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, what)
	assertNoFrame(t, c, "after the refused pickup "+what)
	if _, ok := srv.State.Object(groundID); !ok {
		t.Fatalf("ground item picked up %s", what)
	}
}

// TestPickupWhileTeleportingAnswersActionFailed: a pickup click between a
// teleport and Appearing is refused, not queued: once the teleport ends the
// item still lies there, and a new click picks it up.
func TestPickupWhileTeleportingAnswersActionFailed(t *testing.T) {
	t.Parallel()
	srv, objID, groundID := bootDenyAIPicker(t)
	if !onlineLivePlayer(t, srv, objID).SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported no change")
	}

	assertDeniedPickup(t, srv, groundID, "while teleporting")

	onlineLivePlayer(t, srv, objID).SetTeleporting(false)
	srv.SettlePickup(t, objID)
	assertNoFrame(t, srv.Client, "after the teleport ended")
	if _, ok := srv.State.Object(groundID); !ok {
		t.Fatal("the refused pickup ran once the teleport ended")
	}
	c := srv.Client
	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "pickup release")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, "GetItem")
}

// TestPickupWhileImmobileUntilAttackedAnswersActionFailed: the same refusal
// while an ImmobileUntilAttacked effect holds the player.
func TestPickupWhileImmobileUntilAttackedAnswersActionFailed(t *testing.T) {
	t.Parallel()
	srv, objID, groundID := bootDenyAIPicker(t)
	landImmobileUntilAttacked(t, srv, objID)

	assertDeniedPickup(t, srv, groundID, "while immobile until attacked")
}

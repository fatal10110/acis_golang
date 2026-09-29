package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// onPlayerQueue runs fn on the online player's actor queue and waits for it,
// staging an AI wake-up no client packet reaches.
func onPlayerQueue(t *testing.T, srv *gameservertest.Server, objID int32, fn func(*player.Character)) {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	done := make(chan struct{})
	if !pc.Queue().Post(func() { fn(pc); close(done) }) {
		t.Fatal("post to player queue: queue closed")
	}
	<-done
}

// TestMidSwingToggleEndsTheAttack pins the toggle taking the attack
// intention's place: a toggle requested mid-swing waits for the swing, turns
// on, then the character goes idle like any cast without nextActionAttack
// ends, and no further swing starts.
func TestMidSwingToggleEndsTheAttack(t *testing.T) {
	t.Parallel()
	srv, pc := bootMidSwing(t)
	c := srv.Client

	c.Send(encodeRequestMagicSkillUse(midSwingToggleID, false, false))
	assertFrameOpcode(t, mustRead(t, c, "queued toggle ActionFailed"), serverpackets.OpcodeActionFailed, "queued toggle ActionFailed")
	srv.AdvanceUntil(t, "queued toggle on", func() bool { return toggleOn(pc) })
	assertNoAttackFor(t, c, 3*time.Second, "after the toggle turned on")
}

// TestFailedQueuedSkillDoesNotReviveTheAttack pins the queued skill taking
// the place of an attack requested mid-cast: when the skill's resume fails
// at the cast's end, a later wake-up of the character's AI does not start
// the attack it replaced.
func TestFailedQueuedSkillDoesNotReviveTheAttack(t *testing.T) {
	t.Parallel()
	srv, pc, hostileID := bootMidCastBesideHostile(t)
	c := srv.Client
	objID := pc.ObjectID()

	requestAttackMidCast(t, c, hostileID)
	c.Send(encodeRequestMagicSkillUse(queueNextSkillID, false, false))
	assertFrameOpcode(t, mustRead(t, c, "queued skill ActionFailed"), serverpackets.OpcodeActionFailed, "queued skill ActionFailed")
	// Put the queued skill on reuse: its resume fails the pre-attempt gate.
	pc.DisableSkill(cast.ReuseKey(queueCastSkill(queueNextSkillID)), 10*time.Minute)

	srv.AdvanceUntil(t, "cast end", func() bool { return !pc.CastingNow() })
	assertSystemMessageSkillFrame(t, readUntil(t, c, serverpackets.OpcodeSystemMessage, "resume reuse rejection")[0], serverpackets.SystemMessageS1PreparedForReuse, queueNextSkillID, 1)

	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.WakeAI() })
	assertNoAttackFor(t, c, 2*time.Second, "after the AI woke up")
	if pc.CastingNow() {
		t.Fatal("queued skill on reuse started")
	}
}

// bootBowAttacking equips a bow and arrows, then attacks the fixture monster
// and returns once its first shot's Attack is out, with the shot in flight.
func bootBowAttacking(t *testing.T) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a bow shot open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	bow := srv.GiveItem(t, objID, bowTemplateID, 1)
	arrows := srv.GiveItem(t, objID, woodenArrowTemplateID, bowArrowStack)
	startInWorld(t, c)
	equipAndFlush(t, srv, c, bow)
	equipAndFlush(t, srv, c, arrows)

	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAttackRequest(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	readUntil(t, c, serverpackets.OpcodeAttack, "first bow shot")
	return srv, hostile.ObjectID()
}

// readUntilAttackCountingActionFailed reads frames up to the next Attack and
// returns how many ActionFailed frames came before it.
func readUntilAttackCountingActionFailed(t *testing.T, c *scriptedClient, what string) int {
	t.Helper()
	failed := 0
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatalf("%s never arrived", what)
		}
		switch frame[0] {
		case serverpackets.OpcodeAttack:
			return failed
		case serverpackets.OpcodeActionFailed:
			failed++
		}
	}
	t.Fatalf("%s not found within 100 frames", what)
	return 0
}

// TestBowReclickMidShotFailsAgainAtShotEnd pins the attack queued behind a
// bow shot: a re-click mid-shot answers ActionFailed, the shot's end finds
// the bow still reloading and answers a second ActionFailed, and the next
// shot still fires once the reuse is over.
func TestBowReclickMidShotFailsAgainAtShotEnd(t *testing.T) {
	t.Parallel()
	srv, hostileID := bootBowAttacking(t)
	c := srv.Client

	c.Send(encodeAttackRequest(hostileID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	readUntil(t, c, serverpackets.OpcodeActionFailed, "re-click ActionFailed")
	if got := readUntilAttackCountingActionFailed(t, c, "next bow shot"); got != 1 {
		t.Fatalf("ActionFailed between the re-click and the next shot = %d, want 1 (the shot's end)", got)
	}
}

// TestBowShotWithoutReclickSendsNoActionFailed is the other half: with
// nothing queued behind it, a shot's end re-thinks nothing, so shots follow
// one another with no ActionFailed between them.
func TestBowShotWithoutReclickSendsNoActionFailed(t *testing.T) {
	t.Parallel()
	srv, _ := bootBowAttacking(t)
	if got := readUntilAttackCountingActionFailed(t, srv.Client, "next bow shot"); got != 0 {
		t.Fatalf("ActionFailed between two shots = %d, want 0", got)
	}
}

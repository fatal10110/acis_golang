package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A stopped cast reaches the AI while it still counts as in flight
// (CreatureCast.stop, CreatureCast.java:404-434: FINISHED_CASTING is
// notified before _isCastingNow is cleared). PlayableAI.onEvtFinishedCasting
// (PlayableAI.java:43-63) runs the queued intention, or with nothing queued
// the stopped cast's nextActionAttack follow-up, through doIntention /
// doAttackIntention, and PlayerAI.thinkAttack (PlayerAI.java:169-215) finds
// the caster still casting: an in-range target is re-queued and answered
// ActionFailed, an out-of-range one is walked to (MoveToPawn). Then
// PlayableCast.stop (PlayableCast.java:101-107) runs tryToIdle
// (PlayableAI.java:354-371): nothing is busy any more, so doIdleIntention
// drops the re-queued attack and stops the walk (CreatureMove.stop,
// CreatureMove.java:452-463, broadcasts StopMove). PlayerCast.stop
// (PlayerCast.java:381-387) answers its own ActionFailed last, and
// CreatureCast.interrupt (CreatureCast.java:439-446) sends
// CASTING_INTERRUPTED after the whole stop. No swing starts.

// stopCastOnQueue stops the player's cast on its own queue the way Mute,
// PhysicalMute, SilenceMagicPhysical and RemoveTarget do
// (Character.StopCast).
func stopCastOnQueue(t *testing.T, pc *player.Character) {
	t.Helper()
	done := make(chan struct{})
	if !pc.Queue().Post(func() { pc.StopCast(); close(done) }) {
		t.Fatal("post to player queue: queue closed")
	}
	<-done
}

// assertOpcodes fails unless frames carry exactly want, in order.
func assertOpcodes(t *testing.T, frames [][]byte, want []byte, what string) {
	t.Helper()
	got := make([]byte, len(frames))
	for i, frame := range frames {
		got[i] = frame[0]
	}
	if string(got) != string(want) {
		t.Fatalf("%s sent opcodes %x, want %x", what, got, want)
	}
}

// assertStaysIdle proves the player went idle: nothing swings or walks,
// and an AI wake-up thinks no intention left behind.
func assertStaysIdle(t *testing.T, srv *gameservertest.Server, pc *player.Character) {
	t.Helper()
	c := srv.Client
	x, y, z := srv.PlayerPosition(t, pc.ObjectID())
	assertNoAttackFor(t, c, 2*time.Second, "after the stop")
	onPlayerQueue(t, srv, pc.ObjectID(), func(pc *player.Character) { pc.WakeAI() })
	assertOpcodes(t, framesUntilQuiet(c), nil, "an AI wake-up after the stop")
	if nx, ny, nz := srv.PlayerPosition(t, pc.ObjectID()); nx != x || ny != y || nz != z {
		t.Fatalf("player moved after the stop: (%d,%d,%d) -> (%d,%d,%d)", x, y, z, nx, ny, nz)
	}
}

var (
	stopReply      = []byte{serverpackets.OpcodeMagicSkillCanceled, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}
	interruptReply = []byte{serverpackets.OpcodeMagicSkillCanceled, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed, serverpackets.OpcodeSystemMessage}
)

// TestCastStopDropsQueuedAttack: an attack clicked mid-cast on a monster in
// reach, then the cast stopped (StopCast, the Mute path), answers
// MagicSkillCanceled and two ActionFailed (the think's and the stop's) and
// never swings.
func TestCastStopDropsQueuedAttack(t *testing.T) {
	t.Parallel()
	srv, pc, hostileID := bootMidCastBesideHostile(t)
	c := srv.Client
	requestAttackMidCast(t, c, hostileID)

	stopCastOnQueue(t, pc)
	assertOpcodes(t, framesUntilQuiet(c), stopReply, "a stop with an attack queued")
	assertStaysIdle(t, srv, pc)
}

// TestCastInterruptDropsQueuedAttack is the window-gated interrupt (a damage
// break, an abort-cast effect): the same reply, CASTING_INTERRUPTED last.
func TestCastInterruptDropsQueuedAttack(t *testing.T) {
	t.Parallel()
	srv, pc, hostileID := bootMidCastBesideHostile(t)
	c := srv.Client
	requestAttackMidCast(t, c, hostileID)

	onPlayerQueue(t, srv, pc.ObjectID(), func(pc *player.Character) { pc.InterruptCast() })
	frames := framesUntilQuiet(c)
	assertOpcodes(t, frames, interruptReply, "an interrupt with an attack queued")
	if got := wireReader(frames[3][1:]).ReadInt32(); got != serverpackets.SystemMessageCastingInterrupted {
		t.Fatalf("interrupt system message id = %d, want CASTING_INTERRUPTED %d", got, serverpackets.SystemMessageCastingInterrupted)
	}
	assertStaysIdle(t, srv, pc)
}

// TestEscCastCancelDropsQueuedAttack is Esc mid-cast: RequestTargetCancel's
// CANCEL stops the cast (PlayerAI.onEvtCancel, PlayerAI.java:161-167) and
// then idles, silently.
func TestEscCastCancelDropsQueuedAttack(t *testing.T) {
	t.Parallel()
	srv, pc, hostileID := bootMidCastBesideHostile(t)
	c := srv.Client
	requestAttackMidCast(t, c, hostileID)

	c.Send(encodeRequestTargetCancel(0))
	assertOpcodes(t, framesUntilQuiet(c), stopReply, "Esc with an attack queued")
	assertStaysIdle(t, srv, pc)
}

// TestCastStopSkipsNextActionAttackFollowUp: a nextActionAttack skill
// stopped mid-cast on a monster in reach thinks its follow-up attack once,
// still casting, and is answered like a queued attack: no swing.
func TestCastStopSkipsNextActionAttackFollowUp(t *testing.T) {
	t.Parallel()
	srv, pc, _, _ := bootMidCastWith(t, queueFollowUpSkillID)
	c := srv.Client

	stopCastOnQueue(t, pc)
	assertOpcodes(t, framesUntilQuiet(c), stopReply, "a stopped nextActionAttack cast")
	assertStaysIdle(t, srv, pc)
}

// TestCastStopWalksThenStopsForQueuedAttackOutOfReach: an attack queued on
// a monster out of weapon reach starts its approach (MoveToPawn) during the
// stop, and the stop's idle stops it at once (StopMove), before the stop's
// own ActionFailed. The player never leaves its spot.
func TestCastStopWalksThenStopsForQueuedAttackOutOfReach(t *testing.T) {
	t.Parallel()
	srv, pc, _ := bootMidCastBesideHostile(t)
	c := srv.Client
	far := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 400, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	// The far monster becomes the selection mid-cast; the second click
	// queues the attack on it.
	c.Send(encodeAction(far.ObjectID(), hostileX+400, hostileY, hostileZ, false))
	readUntil(t, c, serverpackets.OpcodeMyTargetSelected, "far monster selection")
	drainUntilQuiet(t, c)
	c.Send(encodeAction(far.ObjectID(), hostileX+400, hostileY, hostileZ, false))
	assertFrameOpcode(t, mustRead(t, c, "mid-cast attack ActionFailed"), serverpackets.OpcodeActionFailed, "mid-cast attack ActionFailed")

	stopCastOnQueue(t, pc)
	assertOpcodes(t, framesUntilQuiet(c), []byte{
		serverpackets.OpcodeMagicSkillCanceled, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeStopMove, serverpackets.OpcodeActionFailed,
	}, "a stop with an out-of-reach attack queued")
	assertStaysIdle(t, srv, pc)
}

// TestCastStopWalksThenStopsForQueuedMove: a walk clicked mid-cast runs as
// the queued MOVE_TO during the stop (PlayableAI.thinkMoveTo,
// PlayableAI.java:186-196) and the stop's idle stops it: MoveToLocation,
// StopMove, then the stop's ActionFailed.
func TestCastStopWalksThenStopsForQueuedMove(t *testing.T) {
	t.Parallel()
	srv, pc, _ := bootMidCastBesideHostile(t)
	c := srv.Client
	c.Send(encodeMoveBackwardToLocation(int32(playerOrigin.X)-300, int32(playerOrigin.Y), int32(playerOrigin.Z)))
	expectGroundClickAck(t, c)
	assertFrameOpcode(t, mustRead(t, c, "mid-cast walk ActionFailed"), serverpackets.OpcodeActionFailed, "mid-cast walk ActionFailed")

	stopCastOnQueue(t, pc)
	assertOpcodes(t, framesUntilQuiet(c), []byte{
		serverpackets.OpcodeMagicSkillCanceled, serverpackets.OpcodeMoveToLocation, serverpackets.OpcodeStopMove, serverpackets.OpcodeActionFailed,
	}, "a stop with a walk queued")
	assertStaysIdle(t, srv, pc)
}

// Teleport mid-cast. Creature.teleportTo (Creature.java:386-429) marks the
// player teleporting, so denyAiAction holds (Creature.java:636-639), then
// runs abortAll (Creature.java:1298-1306). The attack stop answers twice:
// PlayableAttack.stop's tryToIdle under denyAiAction (PlayableAI.java:
// 354-360) and PlayerAttack.stop's own ActionFailed (PlayerAttack.java:
// 58-63). The cast stop broadcasts MagicSkillCanceled; its FINISHED_CASTING
// runs the queued intention (or a nextActionAttack follow-up), whose think
// is denied and answers ActionFailed; then tryToIdle answers ActionFailed,
// leaving the next intention alone, and PlayerCast.stop answers its own.
// abortAll(true) then resets the selected monster (Player.setTarget(null),
// Player.java:2497-2505): ActionFailed, then TargetUnselected.
// TeleportToLocation follows, and the TELEPORTED event's doIdleIntention
// (CreatureAI.java:90-93) drops whatever was still queued.

// teleportOpcodes teleports the player in place on its own queue and
// returns the opcodes of every frame up to and including
// TeleportToLocation.
func teleportOpcodes(t *testing.T, srv *gameservertest.Server, pc *player.Character) []byte {
	t.Helper()
	x, y, z := srv.PlayerPosition(t, pc.ObjectID())
	onPlayerQueue(t, srv, pc.ObjectID(), func(pc *player.Character) { pc.TeleportTo(x, y, z, 0) })
	var got []byte
	for _, frame := range framesUntilQuiet(srv.Client) {
		got = append(got, frame[0])
		if frame[0] == serverpackets.OpcodeTeleportToLocation {
			return got
		}
	}
	t.Fatalf("teleport sent no TeleportToLocation: opcodes %x", got)
	return nil
}

func TestTeleportMidCastActionFailedCounts(t *testing.T) {
	t.Parallel()
	const (
		af  byte = serverpackets.OpcodeActionFailed
		msc byte = serverpackets.OpcodeMagicSkillCanceled
		tu  byte = serverpackets.OpcodeTargetUnselected
		ttl byte = serverpackets.OpcodeTeleportToLocation
	)
	cases := []struct {
		name  string
		long  int32
		queue func(t *testing.T, c *scriptedClient, hostileID int32)
		want  []byte
	}{
		{name: "nothing queued", long: queueLongSkillID, want: []byte{af, af, msc, af, af, af, tu, ttl}},
		{name: "attack queued", long: queueLongSkillID, queue: requestAttackMidCast, want: []byte{af, af, msc, af, af, af, af, tu, ttl}},
		{name: "walk queued", long: queueLongSkillID, queue: func(t *testing.T, c *scriptedClient, _ int32) {
			c.Send(encodeMoveBackwardToLocation(int32(playerOrigin.X)-300, int32(playerOrigin.Y), int32(playerOrigin.Z)))
			expectGroundClickAck(t, c)
			assertFrameOpcode(t, mustRead(t, c, "mid-cast walk ActionFailed"), af, "mid-cast walk ActionFailed")
		}, want: []byte{af, af, msc, af, af, af, af, tu, ttl}},
		{name: "nextActionAttack cast", long: queueFollowUpSkillID, want: []byte{af, af, msc, af, af, af, af, tu, ttl}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, pc, hostileID, _ := bootMidCastWith(t, tc.long)
			c := srv.Client
			if tc.queue != nil {
				tc.queue(t, c, hostileID)
			}
			got := teleportOpcodes(t, srv, pc)
			if string(got) != string(tc.want) {
				t.Fatalf("teleport mid-cast sent opcodes %x up to TeleportToLocation, want %x", got, tc.want)
			}
			c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
			drainUntilQuiet(t, c)
			assertNoAttackFor(t, c, 2*time.Second, "after the teleport")
			if pc.CastingNow() {
				t.Fatal("a cast is in flight after the teleport")
			}
		})
	}
}

// TestTeleportOutsideCastActionFailedCounts: with no cast in flight the
// same abortAll answers the attack stop's two ActionFailed, the cast stop's
// two and the target reset's one (Player.setTarget(null) answers even an
// empty selection, Player.java:2497-2499), and no MagicSkillCanceled.
func TestTeleportOutsideCastActionFailedCounts(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	const af, ttl byte = serverpackets.OpcodeActionFailed, serverpackets.OpcodeTeleportToLocation
	got := teleportOpcodes(t, srv, onlinePlayer(t, srv, srv.SoleObjectID(t)))
	if want := []byte{af, af, af, af, af, ttl}; string(got) != string(want) {
		t.Fatalf("teleport outside a cast sent opcodes %x up to TeleportToLocation, want %x", got, want)
	}
}

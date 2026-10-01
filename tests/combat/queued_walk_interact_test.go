package combat

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// walkAway is the point the queued walk heads for: 1000 units north of the
// fixture player, well inside the request's 9900 cap.
const walkAwayX, walkAwayY, walkAwayZ = 10, 1020, 30

// isOwnFrame reports whether frame is an opcode frame whose first field is
// objID, as MoveToLocation, StopMove and Attack carry their actor.
func isOwnFrame(frame []byte, opcode byte, objID int32) bool {
	return len(frame) >= 5 && frame[0] == opcode && wireReader(frame[1:]).ReadInt32() == objID
}

// readUntilOwnMove reads frames for up to d until objID's own MoveToLocation,
// returning the frames read before it and the move itself.
func readUntilOwnMove(t *testing.T, c *scriptedClient, objID int32, d time.Duration, what string) (before [][]byte, move []byte) {
	t.Helper()
	for end := c.Now().Add(d); c.Now().Before(end); {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			continue
		}
		if isOwnFrame(frame, serverpackets.OpcodeMoveToLocation, objID) {
			return before, frame
		}
		before = append(before, frame)
	}
	t.Fatalf("%s: the player's MoveToLocation never arrived", what)
	return nil, nil
}

// assertMoveTo asserts move heads for (x, y).
func assertMoveTo(t *testing.T, move []byte, x, y int32, what string) {
	t.Helper()
	r := wireReader(move[1:])
	r.ReadInt32()
	if gx, gy := r.ReadInt32(), r.ReadInt32(); gx != x || gy != y {
		t.Fatalf("%s: MoveToLocation destination = (%d,%d), want (%d,%d)", what, gx, gy, x, y)
	}
}

// TestWalkReSteeredMidWalkSendsNoStopMove pins MoveBackwardToLocation →
// PlayableAI.tryToMoveTo → thinkMoveTo → PlayerMove.moveToLocation
// (PlayerMove.java:111-200): a new walk requested while one is under way
// re-steers it with MoveToLocation alone, never a StopMove first.
func TestWalkReSteeredMidWalkSendsNoStopMove(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)

	c.Send(encodeMoveBackwardToLocation(1010, 20, 30))
	_, first := readUntilOwnMove(t, c, objID, 2*time.Second, "first walk")
	assertMoveTo(t, first, 1010, 20, "first walk")

	c.Send(encodeMoveBackwardToLocation(walkAwayX, walkAwayY, walkAwayZ))
	before, second := readUntilOwnMove(t, c, objID, 2*time.Second, "re-steered walk")
	assertMoveTo(t, second, walkAwayX, walkAwayY, "re-steered walk")
	for _, frame := range before {
		if isOwnFrame(frame, serverpackets.OpcodeStopMove, objID) || frame[0] == serverpackets.OpcodeActionFailed {
			t.Fatalf("re-steered walk sent opcode %#x before its MoveToLocation", frame[0])
		}
	}
	for _, frame := range srv.ReadQueued(t, c) {
		if isOwnFrame(frame, serverpackets.OpcodeStopMove, objID) {
			t.Fatal("re-steered walk sent StopMove after its MoveToLocation")
		}
	}
}

// TestWalkRequestedMidSwingRunsAtSwingEnd pins PlayableAI.tryToMoveTo
// (PlayableAI.java:392-409) for a swing in flight: the walk is answered
// ActionFailed and kept as the next intention, the swing lands, and
// PlayableAI.onEvtFinishedAttack (PlayableAI.java:66-76) walks in place of
// the attack, which does not swing again.
func TestWalkRequestedMidSwingRunsAtSwingEnd(t *testing.T) {
	t.Parallel()
	s := bootMidSwingPickup(t, 0)

	s.c.Send(encodeMoveBackwardToLocation(walkAwayX, walkAwayY, walkAwayZ))
	assertFrameOpcode(t, s.c.Read(), serverpackets.OpcodeActionFailed, "mid-swing walk ActionFailed")

	before, move := readUntilOwnMove(t, s.c, s.objID, 3*time.Second, "queued walk")
	assertMoveTo(t, move, walkAwayX, walkAwayY, "queued walk")
	landed := false
	for _, frame := range before {
		switch {
		case frame[0] == serverpackets.OpcodeSystemMessage && slices.Contains(damageFeedbackIDs, wireReader(frame[1:]).ReadInt32()):
			landed = true
		case isOwnFrame(frame, serverpackets.OpcodeAttack, s.objID):
			t.Fatal("the player swung again before the queued walk")
		case isOwnFrame(frame, serverpackets.OpcodeStopMove, s.objID):
			t.Fatal("the queued walk sent StopMove")
		}
	}
	if !landed {
		t.Fatal("the queued walk started before the swing landed")
	}
	assertNoSwingBy(t, s.c, s.objID, 2*time.Second, "after the queued walk")
}

// TestWalkRequestedMidCastRunsAtCastEnd is the same for a cast in flight:
// the walk is answered ActionFailed, and PlayableAI.onEvtFinishedCasting
// (PlayableAI.java:43-64) walks once the cast has launched.
func TestWalkRequestedMidCastRunsAtCastEnd(t *testing.T) {
	t.Parallel()
	srv, pc, _ := bootMidCastBesideHostile(t)
	c := srv.Client

	c.Send(encodeMoveBackwardToLocation(walkAwayX, walkAwayY, walkAwayZ))
	assertFrameOpcode(t, mustRead(t, c, "mid-cast walk ActionFailed"), serverpackets.OpcodeActionFailed, "mid-cast walk ActionFailed")
	if !pc.CastingNow() {
		t.Fatal("the walk request ended the cast")
	}

	before, move := readUntilOwnMove(t, c, pc.ObjectID(), 2*queueHitTime*time.Millisecond, "queued walk")
	assertMoveTo(t, move, walkAwayX, walkAwayY, "queued walk")
	launched := false
	for _, frame := range before {
		switch frame[0] {
		case serverpackets.OpcodeMagicSkillLaunched:
			launched = true
		case serverpackets.OpcodeMagicSkillCanceled:
			t.Fatal("the walk request cancelled the cast")
		}
	}
	if !launched {
		t.Fatal("the queued walk started before MagicSkillLaunched")
	}
}

// spawnThrone places a type-1 throne on the fixture player and consumes its
// StaticObjectInfo.
func spawnThrone(t *testing.T, srv *gameservertest.Server) *staticobject.Object {
	t.Helper()
	throne, err := staticobject.NewObject(srv.NewObjectID(), &staticobject.Template{
		ID: 24180017, Location: playerOrigin, Type: staticobject.ChairType,
	})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}
	srv.State.Spawn(throne, playerOrigin.X, playerOrigin.Y, playerOrigin.Z, 0)
	readUntil(t, srv.Client, serverpackets.OpcodeStaticObjectInfo, "throne StaticObjectInfo")
	return throne
}

// clickThroneQueued selects throne, then clicks it again while a swing or
// cast holds the player: the click is answered ActionFailed alone.
func clickThroneQueued(t *testing.T, srv *gameservertest.Server, throne *staticobject.Object, what string) {
	t.Helper()
	c := srv.Client
	x, y, z := int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z)
	c.Send(encodeAction(throne.ObjectID(), x, y, z, false))
	readUntil(t, c, serverpackets.OpcodeMyTargetSelected, "throne selected "+what)
	c.Send(encodeAction(throne.ObjectID(), x, y, z, false))
	frames := srv.ReadQueued(t, c)
	failed := 0
	for _, frame := range frames {
		switch frame[0] {
		case serverpackets.OpcodeActionFailed:
			failed++
		case serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeChairSit:
			t.Fatalf("throne click %s sat the player at once: opcode %#x", what, frame[0])
		}
	}
	if failed != 1 {
		t.Fatalf("throne click %s answered %d ActionFailed, want 1", what, failed)
	}
}

// assertThroneInteract reads for d and checks the queued throne interact
// ran as PlayerAI.thinkInteract (PlayerAI.java:413-461) does for a throne:
// ActionFailed only after an after frame, and no sit or claim.
func assertThroneInteract(t *testing.T, c *scriptedClient, throne *staticobject.Object, after byte, d time.Duration) {
	t.Helper()
	seen, released := false, false
	for end := c.Now().Add(d); c.Now().Before(end); {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			continue
		}
		switch frame[0] {
		case after:
			seen = true
		case serverpackets.OpcodeActionFailed:
			if !seen {
				t.Fatalf("queued throne interact answered before opcode %#x", after)
			}
			released = true
		case serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeChairSit:
			t.Fatalf("queued throne interact sat the player: opcode %#x", frame[0])
		}
	}
	if !released {
		t.Fatal("queued throne interact never answered ActionFailed")
	}
	if throne.Busy() {
		t.Fatal("a throne click claimed the throne")
	}
}

// TestThroneClickMidSwingInteractsAtSwingEnd pins StaticObject.onAction
// (StaticObject.java:37-44) → PlayableAI.tryToInteract (PlayableAI.java:
// 373-390) for a swing in flight: the second click is answered ActionFailed
// and queued, and the swing's end runs the interact in place of the attack.
// A throne's interact only releases the click: it never sits on or claims
// the throne, and the attack does not swing again.
func TestThroneClickMidSwingInteractsAtSwingEnd(t *testing.T) {
	t.Parallel()
	s := bootMidSwingPickup(t, 0)
	throne := spawnThrone(t, s.srv)

	clickThroneQueued(t, s.srv, throne, "mid-swing")
	var swungAgain bool
	for end := s.c.Now().Add(3 * time.Second); s.c.Now().Before(end); {
		frame := s.c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			continue
		}
		switch {
		case frame[0] == serverpackets.OpcodeActionFailed:
			if throne.Busy() {
				t.Fatal("a throne click claimed the throne")
			}
			assertNoSwingBy(t, s.c, s.objID, 2*time.Second, "after the queued throne interact")
			return
		case frame[0] == serverpackets.OpcodeChangeWaitType, frame[0] == serverpackets.OpcodeChairSit:
			t.Fatalf("queued throne interact sat the player: opcode %#x", frame[0])
		case isOwnFrame(frame, serverpackets.OpcodeAttack, s.objID):
			swungAgain = true
		}
	}
	if swungAgain {
		t.Fatal("the player swung again instead of running the queued throne interact")
	}
	t.Fatal("queued throne interact never answered ActionFailed")
}

// TestThroneClickMidCastInteractsAtCastEnd is the same for a cast in
// flight: the queued interact answers ActionFailed only after
// MagicSkillLaunched, and never sits on or claims the throne.
func TestThroneClickMidCastInteractsAtCastEnd(t *testing.T) {
	t.Parallel()
	srv, pc, _ := bootMidCastBesideHostile(t)
	throne := spawnThrone(t, srv)

	clickThroneQueued(t, srv, throne, "mid-cast")
	if !pc.CastingNow() {
		t.Fatal("the throne click ended the cast")
	}
	assertThroneInteract(t, srv.Client, throne, serverpackets.OpcodeMagicSkillLaunched, 2*queueHitTime*time.Millisecond)
	if !pc.Standing() {
		t.Fatal("a throne click left the player not standing")
	}
}

// TestFearedThroneClickKeepsFleeing pins PlayableAI.tryToInteract's deny
// gate (PlayableAI.java:373-379): a feared player's second click on a
// selected throne is answered ActionFailed alone. Nothing is queued or run,
// so no StopMove cuts the fear flee short and the player keeps running.
func TestFearedThroneClickKeepsFleeing(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	throne := spawnThrone(t, srv)
	drainUntilQuiet(t, c)

	x, y, z := int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z)
	c.Send(encodeAction(throne.ObjectID(), x, y, z, false))
	readUntil(t, c, serverpackets.OpcodeMyTargetSelected, "throne selected")
	drainUntilQuiet(t, c)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	player, ok := obj.(interface {
		effectHolder
		IsMoving() bool
	})
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an effect holder", objID, obj)
	}
	landFear(t, hostile, player, curseFearSkillID, 10)
	readMoveOf(t, c, objID, "landing flee")
	if !player.IsMoving() {
		t.Fatal("IsMoving() = false after the landing flee, want a live run")
	}

	c.Send(encodeAction(throne.ObjectID(), x, y, z, false))
	var opcodes []byte
	for _, frame := range srv.ReadQueued(t, c) {
		if isOwnFrame(frame, serverpackets.OpcodeStopMove, objID) || frame[0] == serverpackets.OpcodeActionFailed {
			opcodes = append(opcodes, frame[0])
		}
	}
	if len(opcodes) != 1 || opcodes[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("feared throne click answered opcodes %x, want ActionFailed alone", opcodes)
	}
	if !player.IsMoving() {
		t.Fatal("the feared throne click stopped the fear flee")
	}
	if throne.Busy() {
		t.Fatal("a feared throne click claimed the throne")
	}
}

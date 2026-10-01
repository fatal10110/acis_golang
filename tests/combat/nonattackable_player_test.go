package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// playerFollowOffset is the distance a following player keeps, the offset
// its MoveToPawn carries.
const playerFollowOffset = 70

// clickPair is an attacker (the primary client) and a second player it
// clicks, both in the world.
type clickPair struct {
	srv        *gameservertest.Server
	c, vc      *scriptedClient
	attackerID int32
	victimID   int32
}

// bootClickPair boots a level-1 attacker and dials a level-40 victim with
// karma, both at the spawn origin; every swing of the attacker hits, none
// kills.
func bootClickPair(t *testing.T, karma int, opts ...gameservertest.Option) clickPair {
	t.Helper()
	return bootClickPairSeeded(t, karma, nil, opts...)
}

// bootClickPairSeeded is bootClickPair with seed run on the attacker before
// it enters the world, when its known skills can still be seeded.
func bootClickPairSeeded(t *testing.T, karma int, seed func(srv *gameservertest.Server, attackerID int32), opts ...gameservertest.Option) clickPair {
	t.Helper()
	opts = append([]gameservertest.Option{
		gameservertest.WithCharacter("Attacker", 1, 0),
		gameservertest.WithWantChars(1),
	}, opts...)
	srv := gameservertest.Boot(t, opts...)
	c, attackerID := srv.Client, srv.SoleObjectID(t)
	if seed != nil {
		seed(srv, attackerID)
	}
	victimID := seedPlayer(t, srv, "victim", "Victim", 40, karma)
	vc := srv.DialClient(t, "victim", 1)
	startInWorld(t, c)
	startInWorld(t, vc)
	obj, ok := srv.State.Player(attackerID)
	if !ok {
		t.Fatal("attacker missing from world state")
	}
	done := make(chan struct{})
	if !srv.PlayerQueue(t, attackerID).Post(func() {
		defer close(done)
		obj.(interface{ SetRollSource(func(int) int) }).SetRollSource(func(int) int { return 0 })
	}) {
		t.Fatal("attacker queue closed")
	}
	<-done
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, vc)
	return clickPair{srv: srv, c: c, vc: vc, attackerID: attackerID, victimID: victimID}
}

// walkVictimAway has the victim walk dx along X and waits for it to arrive.
func (p clickPair) walkVictimAway(t *testing.T, dx int) {
	t.Helper()
	x, y, _ := p.srv.PlayerPosition(t, p.victimID)
	p.vc.Send(encodeMoveBackwardToLocation(int32(x+dx), int32(y), int32(playerOrigin.Z)))
	p.srv.AdvanceUntil(t, "victim walk completed", func() bool {
		px, _, _ := p.srv.PlayerPosition(t, p.victimID)
		return px == x+dx
	})
	drainUntilQuiet(t, p.c)
	drainUntilQuiet(t, p.vc)
}

// tickUntil lets time pass in movement-tick steps, running the
// movement-correction ticks the follow task rechecks on, until cond holds.
func (p clickPair) tickUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for passed := time.Duration(0); !cond(); passed += move.PositionUpdateInterval {
		if passed >= 15*time.Second {
			t.Fatalf("%s not observed within 15s", what)
		}
		p.srv.TickPositions()
	}
}

func (p clickPair) victimHealth(t *testing.T) int {
	t.Helper()
	return p.srv.PlayerCurrentHP(t, p.victimID) + p.srv.PlayerCurrentCP(t, p.victimID)
}

// attacksBy counts the Attack frames whose attacker is id.
func attacksBy(frames [][]byte, id int32) int {
	n := 0
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeAttack && wireReader(f[1:]).ReadInt32() == id {
			n++
		}
	}
	return n
}

// followMoveIndex is the index of the first MoveToPawn in which mover walks
// toward target at the follow offset; -1 when none.
func followMoveIndex(frames [][]byte, mover, target int32) int {
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeMoveToPawn {
			continue
		}
		r := wireReader(f[1:])
		if r.ReadInt32() == mover && r.ReadInt32() == target && r.ReadInt32() == playerFollowOffset {
			return i
		}
	}
	return -1
}

// TestForcedAttackOnUnflaggedPlayerSwingsOnce forces (ctrl) an attack on an
// unflagged player outside any PvP zone. The attacker swings once, then goes
// idle without a second Attack and without an ActionFailed.
func TestForcedAttackOnUnflaggedPlayerSwingsOnce(t *testing.T) {
	t.Parallel()
	p := bootClickPair(t, 0)
	selectPlayerTarget(t, p.c, p.victimID)
	drainUntilQuiet(t, p.c)
	full := p.victimHealth(t)

	p.c.Send(encodeAttackRequest(p.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	p.srv.AdvanceUntil(t, "the forced swing landing", func() bool { return p.victimHealth(t) < full })
	p.srv.Advance(t, 3*time.Second)

	frames := readQuiet(p.c)
	if n := attacksBy(frames, p.attackerID); n != 1 {
		t.Fatalf("attacker swings = %d, want 1: opcodes %v", n, opcodes(frames))
	}
	if p.victimHealth(t) <= 0 {
		t.Fatal("the unflagged player died: the swing count proves nothing")
	}
	swing := frameIndex(frames, serverpackets.OpcodeAttack, 0)
	if i := frameIndex(frames[swing:], serverpackets.OpcodeActionFailed, 0); i >= 0 {
		t.Fatalf("idle after the swing answered ActionFailed: opcodes %v", opcodes(frames))
	}
}

// TestAttackOnKarmaPlayerKeepsSwinging has a plain second click attack a
// karma player without force; the attacker keeps swinging at it.
func TestAttackOnKarmaPlayerKeepsSwinging(t *testing.T) {
	t.Parallel()
	p := bootClickPair(t, 500)
	selectPlayerTarget(t, p.c, p.victimID)
	drainUntilQuiet(t, p.c)

	p.c.Send(encodeAction(p.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	p.srv.Advance(t, 4*time.Second)
	if n := attacksBy(readQuiet(p.c), p.attackerID); n < 2 {
		t.Fatalf("attacker swings at the karma player = %d, want it to keep attacking", n)
	}
}

// TestPlainClickOnUnflaggedPlayerFollows has a plain second click land on an
// unflagged player 300 units away. It is answered ActionFailed, then the
// clicker walks after the target with MoveToPawn at the follow offset and
// never attacks it. When the target walks off again, the follow walks after
// it once more.
func TestPlainClickOnUnflaggedPlayerFollows(t *testing.T) {
	t.Parallel()
	p := bootClickPair(t, 0)
	p.walkVictimAway(t, 300)
	selectPlayerTarget(t, p.c, p.victimID)
	drainUntilQuiet(t, p.c)
	full := p.victimHealth(t)

	p.c.Send(encodeAction(p.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, p.c, "follow ActionFailed"), serverpackets.OpcodeActionFailed, "follow ActionFailed")
	near := func() bool {
		// 2D: the follow walk now stops short of the target mid-leg, where
		// the test geodata's echoed height hint has lifted Z every update.
		ax, ay, _ := p.srv.PlayerPosition(t, p.attackerID)
		vx, vy, _ := p.srv.PlayerPosition(t, p.victimID)
		return location.In2DRadius(ax, ay, vx, vy, playerFollowOffset+60)
	}
	p.tickUntil(t, "the follower reaching the target", near)
	frames := readQuiet(p.c)
	if followMoveIndex(frames, p.attackerID, p.victimID) < 0 {
		t.Fatalf("follow walk frames = %v, want MoveToPawn toward the target at offset %d", opcodes(frames), playerFollowOffset)
	}

	p.walkVictimAway(t, 400)
	p.tickUntil(t, "the follower catching up again", near)
	if n := attacksBy(readQuiet(p.c), p.attackerID); n != 0 {
		t.Fatalf("follower swung %d times at the unflagged player", n)
	}
	if p.victimHealth(t) != full {
		t.Fatal("plain click on an unflagged player damaged it")
	}
}

// TestShiftClickOnUnflaggedPlayerStaysPut has the second click held with
// shift: the follow goes idle at once, answered ActionFailed, with no walk.
func TestShiftClickOnUnflaggedPlayerStaysPut(t *testing.T) {
	t.Parallel()
	p := bootClickPair(t, 0)
	p.walkVictimAway(t, 300)
	selectPlayerTarget(t, p.c, p.victimID)
	drainUntilQuiet(t, p.c)

	p.c.Send(encodeAction(p.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), true))
	assertFrameOpcode(t, mustRead(t, p.c, "shift ActionFailed"), serverpackets.OpcodeActionFailed, "shift ActionFailed")
	p.srv.Advance(t, 3*time.Second)
	if frames := readQuiet(p.c); frameIndex(frames, serverpackets.OpcodeMoveToPawn, 0) >= 0 || attacksBy(frames, p.attackerID) != 0 {
		t.Fatalf("shift-click frames = %v, want no walk and no swing", opcodes(frames))
	}
	if x, _, _ := p.srv.PlayerPosition(t, p.attackerID); x != playerOrigin.X {
		t.Fatalf("attacker moved to x=%d on a shift-click", x)
	}
}

// TestPlainClickOnStorePlayerOpensItsStore clicks an unflagged player
// running a sell store within reach: the click is released, the clicker
// faces the store (MoveToPawn at the interaction distance) and is shown its
// (empty) sell list instead of attacking or following it.
func TestPlainClickOnStorePlayerOpensItsStore(t *testing.T) {
	t.Parallel()
	p := bootClickPair(t, 0)
	p.walkVictimAway(t, 50)
	p.srv.SetPlayerOperating(t, p.victimID, true)
	selectPlayerTarget(t, p.c, p.victimID)
	drainUntilQuiet(t, p.c)

	p.c.Send(encodeAction(p.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, p.c, "store ActionFailed"), serverpackets.OpcodeActionFailed, "store ActionFailed")
	face := mustRead(t, p.c, "store MoveToPawn")
	assertFrameOpcode(t, face, serverpackets.OpcodeMoveToPawn, "store MoveToPawn")
	r := wireReader(face[1:])
	if mover, target, distance := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); mover != p.attackerID || target != p.victimID || distance != 150 {
		t.Fatalf("MoveToPawn %d -> %d at %d, want %d -> %d at 150", mover, target, distance, p.attackerID, p.victimID)
	}
	list := mustRead(t, p.c, "PrivateStoreListSell")
	assertFrameOpcode(t, list, serverpackets.OpcodePrivateStoreListSell, "PrivateStoreListSell")
	r = wireReader(list[1:])
	if owner, packaged, _, rows := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); owner != p.victimID || packaged != 0 || rows != 0 {
		t.Fatalf("PrivateStoreListSell owner %d packaged %d rows %d, want %d, 0, 0", owner, packaged, rows, p.victimID)
	}
	p.srv.Advance(t, 2*time.Second)
	if frames := readQuiet(p.c); attacksBy(frames, p.attackerID) != 0 || followMoveIndex(frames, p.attackerID, p.victimID) >= 0 {
		t.Fatalf("store click frames = %v, want no swing and no follow", opcodes(frames))
	}
}

// TestFollowClickedMidSwingWaitsForTheSwing starts a swing at the fixture
// monster, then clicks an unflagged player twice. The follow is queued with
// ActionFailed; once the swing ends it replaces the attack: the attacker
// walks after the player and never swings at the monster again.
func TestFollowClickedMidSwingWaitsForTheSwing(t *testing.T) {
	t.Parallel()
	p := bootClickPair(t, 0)
	if !p.srv.DrivesClock() {
		t.Skip("holding a swing open needs the driven clock")
	}
	p.walkVictimAway(t, 300)
	hostile := p.srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, p.c)
	targetHostile(t, p.c, hostile.ObjectID())
	p.c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAttackBy(t, p.c, p.attackerID)

	selectPlayerTarget(t, p.c, p.victimID)
	p.c.Send(encodeAction(p.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, p.c, "queued follow ActionFailed"), serverpackets.OpcodeActionFailed, "queued follow ActionFailed")
	if x, _, _ := p.srv.PlayerPosition(t, p.attackerID); x != playerOrigin.X {
		t.Fatalf("attacker walked off mid-swing to x=%d", x)
	}

	p.srv.Advance(t, 3*time.Second)
	frames := readQuiet(p.c)
	if followMoveIndex(frames, p.attackerID, p.victimID) < 0 {
		t.Fatalf("frames after the swing = %v, want the queued follow's MoveToPawn", opcodes(frames))
	}
	if n := attacksBy(frames, p.attackerID); n != 0 {
		t.Fatalf("attacker swung %d more times at the monster, want the follow to replace the attack", n)
	}
}

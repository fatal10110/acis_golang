package combat

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Status ids from StatusType.java: CUR_HP=9, MAX_HP=10, CUR_MP=11,
// CUR_CP=33, MAX_CP=34.
const (
	wantCurHP = 9
	wantMaxHP = 10
	wantCurMP = 11
	wantCurCP = 33
	wantMaxCP = 34
)

// statusFixture is the StatusUpdate layout from StatusUpdate.java:26-38:
// writeC(0x0e), writeD(objectId), writeD(count), then (id, value) int32
// pairs, little-endian.
func statusFixture(objectID int32, pairs ...int32) []byte {
	out := []byte{0x0e}
	out = binary.LittleEndian.AppendUint32(out, uint32(objectID))
	out = binary.LittleEndian.AppendUint32(out, uint32(len(pairs)/2))
	for _, v := range pairs {
		out = binary.LittleEndian.AppendUint32(out, uint32(v))
	}
	return out
}

// statusFramesFor reads until the stream stays quiet and returns every
// StatusUpdate about objectID, and whether a Die for it arrived.
func statusFramesFor(t *testing.T, c *scriptedClient, objectID int32) (statuses [][]byte, died bool) {
	t.Helper()
	for range 100 {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			return statuses, died
		}
		if len(frame) < 5 || wireReader(frame[1:]).ReadInt32() != objectID {
			continue
		}
		switch frame[0] {
		case serverpackets.OpcodeStatusUpdate:
			statuses = append(statuses, frame)
		case serverpackets.OpcodeDie:
			died = true
		}
	}
	t.Fatal("client kept receiving frames after 100 reads")
	return nil, false
}

func assertStatusFrames(t *testing.T, what string, got [][]byte, want ...[]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d StatusUpdate frames %x, want %d %x", what, len(got), got, len(want), want)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("%s: StatusUpdate %d = %x, want %x", what, i, got[i], want[i])
		}
	}
}

// joinWatcher logs a second player into the shared spawn region.
func joinWatcher(t *testing.T, srv *gameservertest.Server) *scriptedClient {
	t.Helper()
	srv.SeedCharacterFor(t, "watcher", "Watcher", 1, 0)
	w := srv.DialClient(t, "watcher", 1)
	startInWorld(t, w)
	drainUntilQuiet(t, srv.Client)
	return w
}

// TestHostileHPStatusFollowsTargetersAndBarSegments pins
// CreatureStatus.broadcastStatusUpdate (CreatureStatus.java:459-469): an
// NPC's HP change reaches only the players targeting it (the status
// listeners Player.setTarget registers, Player.java:2463-2484), carries
// CUR_HP alone, and is gated by needHpUpdate's 352-segment bar
// (CreatureStatus.java:416-454), which is never evaluated while nobody
// targets the NPC.
//
// Expected values follow the reference formula for the fixture's
// calculated max HP 440: interval = 440/352 = 1.25; initial checks
// inc=440, dec=438.75.
func TestHostileHPStatusFollowsTargetersAndBarSegments(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)
	w := joinWatcher(t, srv)
	hostile := srv.SpawnHostileNPC(t)
	id := hostile.ObjectID()
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, w)
	if got := hostile.MaxHP(); got != 440 {
		t.Fatalf("fixture max HP = %d, want 440", got)
	}

	// Unwatched: 437 crosses dec=438.75, but with no listener the gate is
	// skipped, so its checks stay at inc=440, dec=438.75 instead of moving
	// to dec=436.25, inc=437.5.
	hostile.ConsumeHP(3)
	statuses, _ := statusFramesFor(t, c, id)
	assertStatusFrames(t, "unwatched damage", statuses)

	c.Send(encodeAction(id, hostileX, hostileY, hostileZ, false))
	statuses, _ = statusFramesFor(t, c, id)
	assertStatusFrames(t, "target select", statuses, statusFixture(id, wantMaxHP, 440, wantCurHP, 437))
	drainUntilQuiet(t, w)

	// 436.5 <= 438.75: sent as (int) 436; checks move to dec=436.25,
	// inc=437.5. Had the unwatched hit advanced the gate, 436.5 would have
	// stayed silent.
	hostile.ConsumeHP(0.5)
	statuses, _ = statusFramesFor(t, c, id)
	assertStatusFrames(t, "segment crossed", statuses, statusFixture(id, wantCurHP, 436))
	statuses, _ = statusFramesFor(t, w, id)
	assertStatusFrames(t, "non-targeting observer", statuses)

	// 436.3 stays inside (436.25, 437.5): silent.
	hostile.ConsumeHP(0.2)
	statuses, _ = statusFramesFor(t, c, id)
	assertStatusFrames(t, "inside segment", statuses)

	// 436.0 <= 436.25: sent.
	hostile.ConsumeHP(0.3)
	statuses, _ = statusFramesFor(t, c, id)
	assertStatusFrames(t, "next segment", statuses, statusFixture(id, wantCurHP, 436))

	// Death: hp <= 1 always sends, still CUR_HP only and still only to the
	// targeting player; the observer sees Die alone.
	hostile.ConsumeHP(436)
	statuses, died := statusFramesFor(t, c, id)
	if !died || len(statuses) == 0 {
		t.Fatalf("targeting player death: died=%v statuses=%d, want Die and CUR_HP=0 updates", died, len(statuses))
	}
	for _, frame := range statuses {
		assertStatusFrames(t, "death status", [][]byte{frame}, statusFixture(id, wantCurHP, 0))
	}
	statuses, died = statusFramesFor(t, w, id)
	if !died {
		t.Fatal("non-targeting observer missed Die")
	}
	assertStatusFrames(t, "non-targeting observer death", statuses)
}

// TestPlayerStatusGoesOnlyToSelfWithCPAndMP pins
// PlayerStatus.broadcastStatusUpdate (PlayerStatus.java:408-416): a
// player's HP change sends the player itself CUR_HP, CUR_MP, CUR_CP and
// MAX_CP, and nothing to another player targeting it, which only ever gets
// the MAX_HP/CUR_HP snapshot from its own selection (Player.java:2486-2490).
func TestPlayerStatusGoesOnlyToSelfWithCPAndMP(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	w := joinWatcher(t, srv)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world")
	}
	player := obj.(interface {
		CurrentHP() int
		SetSpawnProtection(bool)
		ReduceHP(float64, attackable.Combatant, modelskill.Definition)
	})
	player.SetSpawnProtection(false)
	x, y, z := srv.PlayerPosition(t, objID)
	w.Send(encodeAction(objID, int32(x), int32(y), int32(z), false))
	statuses, _ := statusFramesFor(t, w, objID)
	maxHP, curHP := int32(srv.PlayerMaxHP(t, objID)), int32(player.CurrentHP())
	assertStatusFrames(t, "observer target select", statuses, statusFixture(objID, wantMaxHP, maxHP, wantCurHP, curHP))
	drainUntilQuiet(t, c)

	cp := int32(srv.PlayerCurrentCP(t, objID))
	player.ReduceHP(float64(cp)+1, nil, modelskill.Definition{})
	statuses, _ = statusFramesFor(t, c, objID)
	assertStatusFrames(t, "self", statuses, statusFixture(objID,
		wantCurHP, curHP-1,
		wantCurMP, int32(srv.PlayerCurrentMP(t, objID)),
		wantCurCP, 0,
		wantMaxCP, int32(srv.PlayerMaxCP(t, objID)),
	))
	statuses, _ = statusFramesFor(t, w, objID)
	assertStatusFrames(t, "targeting observer", statuses)

	// Death keeps the same recipients and fields.
	player.ReduceHP(float64(curHP), nil, modelskill.Definition{})
	statuses, died := statusFramesFor(t, c, objID)
	if !died || len(statuses) == 0 {
		t.Fatalf("self death: died=%v statuses=%d, want Die and status updates", died, len(statuses))
	}
	for _, frame := range statuses {
		assertStatusFrames(t, "self death status", [][]byte{frame}, statusFixture(objID,
			wantCurHP, 0,
			wantCurMP, int32(srv.PlayerCurrentMP(t, objID)),
			wantCurCP, 0,
			wantMaxCP, int32(srv.PlayerMaxCP(t, objID)),
		))
	}
	statuses, died = statusFramesFor(t, w, objID)
	if !died {
		t.Fatal("targeting observer missed Die")
	}
	assertStatusFrames(t, "targeting observer death", statuses)
}

// TestHostileHPStatusFramesAreOwnedPerTargeter gives two targeting players
// the same CUR_HP update and proves each received an independent frame.
func TestHostileHPStatusFramesAreOwnedPerTargeter(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)
	w := joinWatcher(t, srv)
	hostile := srv.SpawnHostileNPC(t)
	id := hostile.ObjectID()
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, w)
	for _, client := range []*scriptedClient{c, w} {
		client.Send(encodeAction(id, hostileX, hostileY, hostileZ, false))
	}
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, w)

	// 437 <= dec 438.75: sent.
	hostile.ConsumeHP(3)
	want := statusFixture(id, wantCurHP, 437)
	first, _ := statusFramesFor(t, c, id)
	second, _ := statusFramesFor(t, w, id)
	assertStatusFrames(t, "first targeter", first, want)
	assertStatusFrames(t, "second targeter", second, want)
	first[0][len(first[0])-1] ^= 0xff
	assertStatusFrames(t, "second targeter after mutating first", second, want)
}

// TestHostileSetHPRefreshesTargeterBar pins CreatureStatus.setHp
// (CreatureStatus.java:130-162), the write BalanceLife.java:65 lands on
// every target: it ends in broadcastStatusUpdate even when HP did not move,
// so the NPC's targeters get CUR_HP whenever the bar gate passes, and a
// dead NPC's setHp returns before broadcasting anything.
//
// Fixture max HP 440: interval 1.25, initial checks inc=440, dec=438.75.
func TestHostileSetHPRefreshesTargeterBar(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)
	w := joinWatcher(t, srv)
	hostile := srv.SpawnHostileNPC(t)
	id := hostile.ObjectID()
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, w)
	c.Send(encodeAction(id, hostileX, hostileY, hostileZ, false))
	drainUntilQuiet(t, c)

	// 430 <= dec 438.75: sent; checks move to dec=430, inc=431.25.
	hostile.SetHP(430)
	statuses, _ := statusFramesFor(t, c, id)
	assertStatusFrames(t, "balanced down", statuses, statusFixture(id, wantCurHP, 430))
	statuses, _ = statusFramesFor(t, w, id)
	assertStatusFrames(t, "non-targeting observer", statuses)

	// 430.5 stays inside (430, 431.25): the gate declines.
	hostile.SetHP(430.5)
	statuses, _ = statusFramesFor(t, c, id)
	assertStatusFrames(t, "inside segment", statuses)

	// An unchanged 430.5 still re-runs the gate, which still declines;
	// the full-HP write crosses inc=431.25 and is sent.
	hostile.SetHP(430.5)
	statuses, _ = statusFramesFor(t, c, id)
	assertStatusFrames(t, "unchanged value", statuses)
	hostile.SetHP(10000)
	statuses, _ = statusFramesFor(t, c, id)
	assertStatusFrames(t, "clamped to full", statuses, statusFixture(id, wantCurHP, 440))

	hostile.ConsumeHP(440)
	statusFramesFor(t, c, id)
	hostile.SetHP(200)
	statuses, _ = statusFramesFor(t, c, id)
	assertStatusFrames(t, "dead NPC", statuses)
	if hp := hostile.CurrentHP(); hp != 0 {
		t.Fatalf("dead NPC HP = %d after SetHP, want 0", hp)
	}
}

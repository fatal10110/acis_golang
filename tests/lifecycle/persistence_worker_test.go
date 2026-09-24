package lifecycle

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestStopSavesConnectedPlayer stops the server with a player still in game.
// Stopping the listener detaches every connection, and each handler waits for
// its player's saves before returning, so once serving has stopped the
// characters row holds the final HP and is marked offline.
func TestStopSavesConnectedPlayer(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	startInWorld(t, srv.Client)
	objID := srv.SoleObjectID(t)
	srv.DamagePlayerHP(t, objID, 10)
	wantHP := srv.PlayerCurrentHP(t, objID)

	srv.Stop()

	hp, online := persistedHPAndOnline(t, srv, objID)
	if hp != wantHP || online != 0 {
		t.Fatalf("characters row after stop = hp %d online %d, want hp %d online 0", hp, online, wantHP)
	}
}

// TestRelogMidFightRestoresSavedHP drops the connection right after a monster
// hit lands while the player's persistence lane is backed up, and logs the
// same character back in on a new connection. Selecting the character must
// wait for the old session's queued saves and restore from the saved row, so
// the relogged character carries the post-hit HP rather than the HP the
// character list read before those saves landed.
func TestRelogMidFightRestoresSavedHP(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	startInWorld(t, srv.Client)
	objID := srv.SoleObjectID(t)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world")
	}
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: spawnX + 50, Y: spawnY, Z: spawnZ})
	drainUntilQuiet(t, srv.Client)

	fullHP := srv.PlayerCurrentHP(t, objID)
	attacker.DoAttack(t, obj.(attackable.Combatant))
	hitHP := srv.PlayerCurrentHP(t, objID)
	if hitHP >= fullHP || hitHP <= 0 {
		t.Fatalf("HP after the hit = %d, want a survived hit below %d", hitHP, fullHP)
	}

	release := srv.HoldPersistenceLane(t, objID)
	if err := srv.Client.Close(); err != nil {
		t.Fatal(err)
	}
	srv.AdvanceUntil(t, "player left world", func() bool {
		_, ok := srv.State.Player(objID)
		return !ok
	})
	if hp, _ := persistedHPAndOnline(t, srv, objID); hp == hitHP {
		t.Fatalf("characters row already holds the post-hit HP %d while the lane is held", hp)
	}

	c := srv.DialClient(t, "player1", 1)
	c.Send(encodeRequestGameStart(0))
	// Selection is parked on the held lane.
	c.ExpectNoFrame()
	release()
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "game start SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "game start CharSelected")
	c.Send(encodeEnterWorld())
	// The monster is still in range, so its NpcInfo interleaves the burst.
	drainUntilQuiet(t, c)
	if got := srv.PlayerCurrentHP(t, objID); got != hitHP {
		t.Fatalf("relogged HP = %d, want %d (HP when the connection dropped)", got, hitHP)
	}
}

// TestSelectRefusedWhenQueuedSavesTimeOut backs up a logged-out character's
// persistence lane past the connection's wait budget and selects the
// character again. The selection is refused silently rather than loading
// rows the old session has not written; once the lane drains, a new
// selection goes through.
func TestSelectRefusedWhenQueuedSavesTimeOut(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithPersistWait(200*time.Millisecond),
	)
	startInWorld(t, srv.Client)
	objID := srv.SoleObjectID(t)

	release := srv.HoldPersistenceLane(t, objID)
	if err := srv.Client.Close(); err != nil {
		t.Fatal(err)
	}
	srv.AdvanceUntil(t, "player left world", func() bool {
		_, ok := srv.State.Player(objID)
		return !ok
	})

	c := srv.DialClient(t, "player1", 1)
	c.Send(encodeRequestGameStart(0))
	if frame := c.ReadWithTimeout(time.Second); frame != nil {
		t.Fatalf("selection answered %#x while its queued saves were still held past the wait budget", frame[0])
	}

	release()
	srv.FlushPersistence(t)
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "game start SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "game start CharSelected")
}

func persistedHPAndOnline(t *testing.T, srv *gameservertest.Server, objID int32) (hp, online int) {
	t.Helper()
	if err := srv.DB.QueryRow("SELECT FLOOR(curHp), online FROM characters WHERE obj_Id = ?", objID).Scan(&hp, &online); err != nil {
		t.Fatalf("read characters row: %v", err)
	}
	return hp, online
}

package lifecycle

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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
// hit lands and logs the same character back in: the relogged character
// carries the HP it had when the connection dropped, not its pre-fight HP.
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
	attacker.DoAttack(t, obj.(attackable.Combatant), 5*time.Second)
	hitHP := srv.PlayerCurrentHP(t, objID)
	if hitHP >= fullHP || hitHP <= 0 {
		t.Fatalf("HP after the hit = %d, want a survived hit below %d", hitHP, fullHP)
	}

	if err := srv.Client.Close(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "player left world", func() bool {
		_, ok := srv.State.Player(objID)
		return !ok
	})
	srv.FlushPersistence(t)

	c := srv.DialClient(t, "player1", 1)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	// The monster is still in range, so its NpcInfo interleaves the burst.
	drainUntilQuiet(t, c)
	if got := srv.PlayerCurrentHP(t, objID); got != hitHP {
		t.Fatalf("relogged HP = %d, want %d (HP when the connection dropped)", got, hitHP)
	}
}

func persistedHPAndOnline(t *testing.T, srv *gameservertest.Server, objID int32) (hp, online int) {
	t.Helper()
	if err := srv.DB.QueryRow("SELECT FLOOR(curHp), online FROM characters WHERE obj_Id = ?", objID).Scan(&hp, &online); err != nil {
		t.Fatalf("read characters row: %v", err)
	}
	return hp, online
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("%s not observed within 5s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

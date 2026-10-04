package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// A dropped connection. GameClient.onDisconnection (GameClient.java:
// 201-213) cleans a player in combat or locked up after 15 s, any other
// after 100 ms (cleanMe, GameClient.java:610-614). Until then the character
// stays in the world. A kick or a relog of the account closes the client
// (closeNow, GameClient.java:592-602), which cleans up at once. Cleanup
// drops the attack stance silently (Player.cleanup, Player.java:6292;
// AttackStanceTaskManager.remove, AttackStanceTaskManager.java:86-92), so
// observers see the DeleteObject and no AutoAttackStop.

// bootInStanceObserved boots the observer pair and has a monster hit the
// primary player, which puts it in attack stance.
func bootInStanceObserved(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv, c, observer, objID := bootObserverPair(t)
	hostile := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: spawnOrigin.X + 50, Y: spawnOrigin.Y, Z: spawnOrigin.Z})
	drainQuiet(t, c)
	drainQuiet(t, observer)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	hostile.DoAttack(t, obj.(attackable.Combatant))
	mustReadOpcode(t, observer, serverpackets.OpcodeAutoAttackStart, "observer AutoAttackStart")
	drainQuiet(t, c)
	drainQuiet(t, observer)
	return srv, c, observer, objID
}

// watchLeave reads observer for d and reports whether it saw objID's
// DeleteObject. It fails on an AutoAttackStop for objID when stance is set.
// It reads in short steps: a step lets the clock run, and the detach a due
// delay releases needs a moment of its own to reach the observer.
func watchLeave(t *testing.T, observer *testsupport.ScriptedClient, objID int32, d time.Duration, stance bool) bool {
	t.Helper()
	for end := observer.Now().Add(d); observer.Now().Before(end); {
		frame := observer.ReadWithTimeout(min(50*time.Millisecond, end.Sub(observer.Now())))
		if frame == nil {
			continue
		}
		if len(frame) < 5 || wire.NewReader(frame[1:]).ReadInt32() != objID {
			continue
		}
		switch frame[0] {
		case serverpackets.OpcodeDeleteObject:
			return true
		case serverpackets.OpcodeAutoAttackStop:
			if stance {
				t.Fatal("observer got an AutoAttackStop for the leaving player")
			}
		}
	}
	return false
}

func TestDroppedConnectionInStanceLingersUntilRelog(t *testing.T) {
	t.Parallel()
	srv, c, observer, objID := bootInStanceObserved(t)

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if watchLeave(t, observer, objID, 8*time.Second, true) {
		t.Fatal("a player dropped in attack stance left before the 15 s linger")
	}
	if _, ok := srv.State.Player(objID); !ok {
		t.Fatal("a player dropped in attack stance left the world before the 15 s linger")
	}

	// Logging the account back in evicts the lingering session: the
	// character leaves at once, its stance dropped silently.
	srv.DialClient(t, srv.Account(), 1)
	if !watchLeave(t, observer, objID, 2*time.Second, true) {
		t.Fatal("observer never saw the evicted player leave")
	}
}

func TestDroppedConnectionInStanceLeavesAfterLinger(t *testing.T) {
	t.Parallel()
	srv, c, observer, objID := bootInStanceObserved(t)

	dropped := observer.Now()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	for {
		if _, ok := srv.State.Player(objID); !ok {
			break
		}
		if observer.Now().Sub(dropped) > 30*time.Second {
			t.Fatal("a player dropped in attack stance was still in the world 30 s later")
		}
		srv.Advance(t, 100*time.Millisecond)
	}
	if stayed := observer.Now().Sub(dropped); stayed < 15*time.Second {
		t.Fatalf("a player dropped in attack stance left after %v, want the 15 s linger", stayed)
	}
	// The stance may run out on its own during the linger and send its own
	// AutoAttackStop; only the DeleteObject matters here.
	if !watchLeave(t, observer, objID, 2*time.Second, false) {
		t.Fatal("observer never saw the lingering player leave")
	}
}

func TestDroppedConnectionOutOfCombatLeavesAfterShortDelay(t *testing.T) {
	t.Parallel()
	srv, c, observer, objID := bootObserverPair(t)

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	srv.AdvanceUntil(t, "dropped player out of the world", func() bool {
		_, ok := srv.State.Player(objID)
		return !ok
	})
	if !watchLeave(t, observer, objID, time.Second, false) {
		t.Fatal("observer never saw the dropped player leave")
	}
}

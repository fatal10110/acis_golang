package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestThroneSitStandWhileObserverComesAndGoes runs a throne sit and
// stand on the sitter's queue while an observer teleports in and out of
// its view, each arrival discovering the sitter (CharInfo, then ChairSit
// while it holds the throne) and each departure forgetting it with the
// DeleteObject seated flag. Under -race on the pool executor the
// observer's reads of the sitter's throne must not race its writes.
func TestThroneSitStandWhileObserverComesAndGoes(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	sitterID := srv.SoleObjectID(t)
	second := srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	observer := srv.DialClient(t, "player2", 1)
	enterWorld(t, observer)
	drainQuiet(t, observer)
	drainQuiet(t, c)
	sitOnChair(t, c, observer, spawnChair(t, srv, c, observer))

	watcherObj, ok := srv.State.Player(second.ID)
	if !ok {
		t.Fatal("observer missing from the world")
	}
	watcher, ok := network.OnlineCharacter(watcherObj)
	if !ok {
		t.Fatalf("observer = %T, want an online character", watcherObj)
	}
	away := location.Location{X: spawnOrigin.X + 20_000, Y: spawnOrigin.Y, Z: spawnOrigin.Z}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Stand (the throne frees when the stand-up ends) and sit again
		// on the still-targeted chair, twice over.
		for range 2 {
			c.Send(encodeRequestChangeWaitType(true))
			readSitterOpcode(c, serverpackets.OpcodeChangeWaitType)
			c.Send(encodeRequestChangeWaitType(false))
			readSitterOpcode(c, serverpackets.OpcodeChairSit)
		}
	}()

	// The observer leaves and comes back until the sitter is done, ending
	// in view.
	sawChairSit := false
	for i, sitting := 0, true; sitting || i%2 == 1; i++ {
		select {
		case <-done:
			sitting = false
		default:
		}
		to := spawnOrigin
		if i%2 == 0 {
			to = away
		}
		watcher.TeleportTo(to.X, to.Y, to.Z, 0)
		frames := readObserverUntil(t, observer, serverpackets.OpcodeTeleportToLocation)
		frames = append(frames, appear(t, observer)...)
		if i%2 == 1 && !sitting {
			frames = append(frames, observer.ReadQueued()...)
		} else if f := observer.ReadWithTimeout(15 * time.Millisecond); f != nil {
			// The short read also keeps the hops under the client flood
			// limit while the sitter's transitions run on the wall clock.
			frames = append(frames, f)
		}
		for _, f := range frames {
			if f[0] == serverpackets.OpcodeChairSit {
				sawChairSit = true
			}
		}
	}
	if !sawChairSit {
		t.Fatal("observer never discovered the throne-seated sitter with ChairSit")
	}

	// The sitter ends seated on the throne: a fresh departure forgets it
	// with the stand-then-delete flag.
	watcher.TeleportTo(away.X, away.Y, away.Z, 0)
	if got := readDeleteObjectFlag(t, observer, sitterID); got != 0 {
		t.Fatalf("DeleteObject seated flag = %d, want 0 for a throne-seated sitter", got)
	}
}

// readSitterOpcode reads c until opcode arrives or a quiet 10s passes; the
// main goroutine's checks own the test's failure.
func readSitterOpcode(c *testsupport.ScriptedClient, opcode byte) {
	deadline := c.Now().Add(10 * time.Second)
	for c.Now().Before(deadline) {
		if f := c.ReadWithTimeout(500 * time.Millisecond); f != nil && f[0] == opcode {
			return
		}
	}
}

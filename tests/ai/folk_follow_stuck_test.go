package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestFolkFollowTeleportsAStuckFollowerNextToItsTarget pins the follow step's
// recovery (NpcAI.thinkFollow) through the live AI cycle: a civilian NPC
// sent after a target far off over closed ground counts a failed path for
// each walk it starts and keeps its streak until a path is found. The cycle
// that finds ten in a row stops it and jumps it to within 10 of its target,
// and a new streak starts. It does not jump before the tenth.
func TestFolkFollowTeleportsAStuckFollowerNextToItsTarget(t *testing.T) {
	t.Parallel()
	const (
		cycles   = 600 // a walk is tried on a few of the cycles, so ten tries take a hundred at most; the rest is slack for rolls
		failures = 10
		offset   = 10
	)
	srv, at := bootFolkDesires(t, nil)
	needsDrivenClock(t, srv)
	target := characterOf(t, srv)
	player, ok := srv.State.Player(target.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	geo := &gameservertest.GateGeo{}
	geo.Block()
	f := srv.SpawnFolkNPCAtGeo(t, folkFighterTemplate(folkPatrolID), location.Location{X: at.X + 400, Y: at.Y, Z: at.Z}, geo)
	srv.Settle(t)
	cycle(t, srv)
	if !f.Queue().Post(func() { f.AddFollowDesire(target, 100) }) {
		t.Fatal("the folk's queue is closed")
	}
	srv.Settle(t)

	waited := 0
	for range cycles {
		before := f.GeoPathFailCount()
		cycle(t, srv)
		srv.Advance(t, time.Second)
		srv.Settle(t)
		x, y, _ := f.Position()
		tx, ty, tz := target.Position()
		// Only the jump ends the streak: every path is closed.
		if jumped := before > 0 && f.GeoPathFailCount() == 0; jumped {
			if before != failures {
				t.Fatalf("the follower jumped after %d failed paths, want %d", before, failures)
			}
			if x < tx-offset || x > tx+offset || y < ty-offset || y > ty+offset {
				t.Fatalf("the follower jumped to (%d, %d), want within %d of its target at (%d, %d)", x, y, offset, tx, ty)
			}
			if f.IsMoving() {
				t.Fatal("the follower walks after the jump")
			}
			return
		}
		// The follow step runs on some of the cycles only: the ones at the
		// limit may be the others.
		if before >= failures {
			if waited++; waited > 6 {
				t.Fatalf("the follower kept %d failed paths for seven cycles and did not jump", before)
			}
		}
		// Arrived within its loose radius, it would hold still: send the
		// target off again so it has to try another walk.
		if !f.IsMoving() && location.In2DRadius(x, y, tx, ty, 150) {
			if err := srv.State.Move(player, x+700, y, tz); err != nil {
				t.Fatalf("move the character: %v", err)
			}
		}
	}
	t.Fatalf("the follower never jumped in %d cycles, path failures reached %d", cycles, f.GeoPathFailCount())
}

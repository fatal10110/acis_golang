package ai

import (
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// sweepPatrol is a civilian border patrol spawned 500 to the east of the
// character, with a behavior that records its out-of-territory hook; home
// is its spawn point and out a point outside its territory.
type sweepPatrol struct {
	srv       *gameservertest.Server
	f         *npc.Folk
	player    attackable.Combatant
	log       *folkLog
	home, out location.Location
}

func bootSweepPatrol(t *testing.T) *sweepPatrol {
	t.Helper()
	log := &folkLog{}
	patrol := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{folkPatrolID}, Hooks: script.Hooks{
			OnOutOfTerritory: func(_ *script.Script, _ script.OutOfTerritory) {
				log.add("OUT_OF_TERRITORY")
			},
		}}
	}
	srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkPatrolSweep": patrol})
	home := location.Location{X: at.X + 500, Y: at.Y, Z: at.Z}
	p := &sweepPatrol{
		srv:    srv,
		player: characterOf(t, srv),
		log:    log,
		home:   home,
		out:    location.Location{X: home.X, Y: home.Y + 300, Z: home.Z},
	}
	p.f = srv.SpawnFolkNPCAt(t, folkFighterTemplate(folkPatrolID), home)
	srv.ReadQueued(t, srv.Client)
	return p
}

// hook asserts the out-of-territory hooks run since the last call.
func (p *sweepPatrol) hooks(t *testing.T, when string, want ...string) {
	t.Helper()
	if got := p.log.take(); !slices.Equal(got, want) {
		t.Fatalf("hooks %s = %q, want %q", when, got, want)
	}
}

// hated asserts whether the patrol still hates the character and still
// holds its attack desire on it.
func (p *sweepPatrol) hated(t *testing.T, when string, want bool) {
	t.Helper()
	c, ok := p.f.MostHated()
	hates := ok && c.ObjectID() == p.player.ObjectID()
	desire := p.f.Desires().Has(&actorai.Desire{Kind: actorai.IntentionAttack, FinalTarget: p.player})
	if hates != want || desire != want {
		t.Fatalf("%s: hates the character = %v, attack desire on it = %v; want both %v", when, hates, desire, want)
	}
}

// chaseOut has the patrol chase the character to where it stands, outside
// the patrol's territory: the chase's arrival opens the out-of-territory
// hook and arms the stale-aggro sweep.
func (p *sweepPatrol) chaseOut(t *testing.T) {
	t.Helper()
	p.f.AddAttackDesire(p.player, 2000)
	walkUntilArrived(t, p.srv, p.f)
	if p.f.InTerritory() {
		t.Fatal("the chase ended inside the patrol's territory; want outside")
	}
	p.hooks(t, "on the chase's arrival", "OUT_OF_TERRITORY")
}

// walkTo has the patrol walk to to through its AI, without moving the
// clock before the walk starts, and waits for its arrival.
func (p *sweepPatrol) walkTo(t *testing.T, to location.Location) {
	t.Helper()
	if !p.f.AddMoveToDesire(to, 1e6) {
		t.Fatalf("AddMoveToDesire(%+v) = false, want the walk queued", to)
	}
	for i := 0; ; i++ {
		if dest, moving := p.f.MovingTo(); moving && dest == to {
			break
		}
		if i == 5 {
			t.Fatalf("intention = %v after five cycles, want the walk to %+v", p.f.CurrentIntention(), to)
		}
		cycle(t, p.srv)
	}
	walkUntilArrived(t, p.srv, p.f)
}

// teleport jumps the patrol to to: no arrival, so neither the hook nor any
// change to the sweep.
func (p *sweepPatrol) teleport(t *testing.T, to location.Location, inside bool) {
	t.Helper()
	p.f.TeleportTo(to)
	if got := p.f.InTerritory(); got != inside {
		t.Fatalf("in territory after the teleport to %+v = %v, want %v", to, got, inside)
	}
	p.srv.ReadQueued(t, p.srv.Client)
}

// TestFolkSweepSkipsWhileBackInsideWithoutArrival arms the stale-aggro
// sweep with a chase that ends outside, then teleports the patrol home: a
// teleport is no arrival, so the sweep stays armed, but each firing finds
// the patrol inside and keeps an entry well past 90 s. Back outside, again
// by teleport, the next firing stops that entry's hate.
func TestFolkSweepSkipsWhileBackInsideWithoutArrival(t *testing.T) {
	t.Parallel()
	p := bootSweepPatrol(t)
	p.chaseOut(t)
	x, y, z := p.f.Position()
	away := location.Location{X: x, Y: y, Z: z}
	p.teleport(t, p.home, true)

	// Firings at +0.1 s, +10.1 s, ... +90.1 s, all inside, past 90 s since
	// the attack desire stamped the entry.
	p.srv.Advance(t, 95*time.Second)
	p.hated(t, "95 s after the chase, back inside", true)

	p.teleport(t, away, false)
	p.srv.Advance(t, 10*time.Second)
	p.hated(t, "after the firing at +100.1 s, outside again", false)
	p.hooks(t, "over the teleports")
}

// TestFolkArrivalBackInsideDisarmsTheSweep arms the sweep with a walk that
// ends outside, then disarms it with a walk back home. An entry stamped
// after that keeps its hate long past 90 s, even once a teleport puts the
// patrol outside again: no arrival there, so no hook and no sweep.
func TestFolkArrivalBackInsideDisarmsTheSweep(t *testing.T) {
	t.Parallel()
	p := bootSweepPatrol(t)
	p.walkTo(t, p.out)
	p.hooks(t, "on arriving outside", "OUT_OF_TERRITORY")
	p.walkTo(t, p.home)
	p.hooks(t, "on arriving back home")

	// The attack desire starts a chase, which the teleport stops short.
	p.f.AddAttackDesire(p.player, 2000)
	p.teleport(t, p.out, false)
	p.srv.Advance(t, 120*time.Second)
	p.hated(t, "120 s after the entry, outside with the sweep disarmed", true)
	p.hooks(t, "over the teleport")
}

// TestFolkFirstSweepRuns100msAfterTheArrival stamps an entry, keeps the
// patrol inside for 90 s, then walks it outside: the entry is already stale
// at the arrival, and the first firing, 100 ms later and not sooner, stops
// its hate.
func TestFolkFirstSweepRuns100msAfterTheArrival(t *testing.T) {
	t.Parallel()
	p := bootSweepPatrol(t)
	// The attack desire starts a chase, which the teleport stops short.
	p.f.AddAttackDesire(p.player, 2000)
	p.teleport(t, p.home, true)
	p.srv.Advance(t, 90*time.Second)

	p.walkTo(t, p.out)
	p.hooks(t, "on arriving outside", "OUT_OF_TERRITORY")
	p.srv.Advance(t, 99*time.Millisecond)
	p.hated(t, "99 ms after the arrival", true)
	p.srv.Advance(t, time.Millisecond)
	p.hated(t, "100 ms after the arrival", false)
}

// TestFolkSweepStopsHateAtExactly90s re-stamps the entry on the 10 s grid
// the chase's arrival outside set, at the first firing 100 ms after it, or
// 1 ms later. Stamped at the firing, the entry is exactly 90 s old at the
// tenth firing, which stops its hate, and the ninth keeps it. Stamped 1 ms
// later, it is 1 ms short of 90 s at the tenth firing, which keeps it, and
// the eleventh stops its hate.
func TestFolkSweepStopsHateAtExactly90s(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		late  time.Duration
		swept time.Duration
	}{
		{"on the grid", 0, 90 * time.Second},
		{"1 ms late", time.Millisecond, 100*time.Second - time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := bootSweepPatrol(t)
			p.chaseOut(t)
			p.srv.Advance(t, 100*time.Millisecond+tc.late)
			// The patrol hates the character already, so this only
			// re-stamps the entry, without a desire selection.
			p.f.AddAttackDesire(p.player, 1)

			p.srv.Advance(t, tc.swept-time.Millisecond)
			p.hated(t, "1 ms before the sweeping firing", true)
			p.srv.Advance(t, time.Millisecond)
			p.hated(t, "at the sweeping firing", false)
		})
	}
}

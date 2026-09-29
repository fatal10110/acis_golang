package combat

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// mpCPWriterPlayer is the slice of a live player the dead MP/CP writer tests
// drive.
type mpCPWriterPlayer interface {
	Die(attackable.Combatant) bool
	Revive() bool
	CurrentMP() int
	CurrentCP() int
	AddMP(float64) float64
	AddCP(float64) float64
	ReduceMP(float64) float64
	ReduceCurrentMP(int)
	SetCP(float64)
	Queue() *sim.Queue
}

func bootMPCPWriterPlayer(t *testing.T) mpCPWriterPlayer {
	t.Helper()
	_, player := bootHPWriterPlayer(t)
	return player.(mpCPWriterPlayer)
}

// TestDeadPlayerRejectsMPAndCPWriters pins that a corpse's MP and CP stay
// where death left them: a restore, cost or set that passed its liveness
// check before the player died is dropped, while a living or revived player
// still gains and spends them.
func TestDeadPlayerRejectsMPAndCPWriters(t *testing.T) {
	t.Parallel()
	player := bootMPCPWriterPlayer(t)

	assertLiveMPCPWriters(t, player)

	// The liveness checks above are now stale: the player dies before the
	// writes.
	if !player.Die(nil) {
		t.Fatal("Die() = false for a living player")
	}
	mp, cp := player.CurrentMP(), player.CurrentCP()
	if got := player.AddMP(10); got != 0 {
		t.Fatalf("dead AddMP(10) = %v, want 0", got)
	}
	if got := player.ReduceMP(10); got != 0 {
		t.Fatalf("dead ReduceMP(10) = %v, want 0", got)
	}
	player.ReduceCurrentMP(10)
	if got := player.AddCP(10); got != 0 {
		t.Fatalf("dead AddCP(10) = %v, want 0", got)
	}
	player.SetCP(1)
	if gotMP, gotCP := player.CurrentMP(), player.CurrentCP(); gotMP != mp || gotCP != cp {
		t.Fatalf("dead player MP/CP = %d/%d after the writers, want unchanged %d/%d", gotMP, gotCP, mp, cp)
	}

	if !player.Revive() {
		t.Fatal("Revive() = false for a dead player")
	}
	assertLiveMPCPWriters(t, player)
}

// assertLiveMPCPWriters drives every MP and CP writer on a living player and
// checks each one lands.
func assertLiveMPCPWriters(t *testing.T, player mpCPWriterPlayer) {
	t.Helper()
	player.SetCP(5)
	if got := player.CurrentCP(); got != 5 {
		t.Fatalf("live SetCP(5) left CP %d", got)
	}
	if got := player.AddCP(3); got != 3 || player.CurrentCP() != 8 {
		t.Fatalf("live AddCP(3) = %v leaving CP %d, want 3 leaving 8", got, player.CurrentCP())
	}
	mp := player.CurrentMP()
	if mp < 20 {
		t.Fatalf("fixture MP %d leaves no room for the MP writers", mp)
	}
	if got := player.ReduceMP(10); got != 10 || player.CurrentMP() != mp-10 {
		t.Fatalf("live ReduceMP(10) = %v leaving MP %d, want 10 leaving %d", got, player.CurrentMP(), mp-10)
	}
	player.ReduceCurrentMP(5)
	if got := player.CurrentMP(); got != mp-15 {
		t.Fatalf("live ReduceCurrentMP(5) left MP %d, want %d", got, mp-15)
	}
	if got := player.AddMP(5); got != 5 || player.CurrentMP() != mp-10 {
		t.Fatalf("live AddMP(5) = %v leaving MP %d, want 5 leaving %d", got, player.CurrentMP(), mp-10)
	}
}

// TestConcurrentPlayerMPCPWritersCannotChangeCorpse races MP and CP writers
// on other goroutines against a death on the player's own queue; once the
// death lands, no writer may move the corpse's MP or CP.
func TestConcurrentPlayerMPCPWritersCannotChangeCorpse(t *testing.T) {
	t.Parallel()
	const rounds, writers = 20, 4
	player := bootMPCPWriterPlayer(t)

	for round := range rounds {
		player.ReduceMP(20)
		player.SetCP(5)
		var stop atomic.Bool
		var wg sync.WaitGroup
		for i := range writers {
			wg.Go(func() {
				for !stop.Load() {
					if i%2 == 0 {
						player.AddMP(1)
						player.AddCP(1)
					} else {
						player.ReduceMP(1)
						player.SetCP(5)
					}
				}
			})
		}
		var mp, cp int
		onQueue(t, player.Queue(), func() {
			if !player.Die(nil) {
				t.Errorf("round %d: Die() = false for a living player", round)
			}
			mp, cp = player.CurrentMP(), player.CurrentCP()
		})
		// Writers keep running past the death before they stop.
		for range 1000 {
			player.AddMP(1)
			player.ReduceMP(1)
		}
		stop.Store(true)
		wg.Wait()
		if gotMP, gotCP := player.CurrentMP(), player.CurrentCP(); gotMP != mp || gotCP != cp {
			t.Fatalf("round %d: corpse MP/CP = %d/%d after concurrent writers, want %d/%d at death", round, gotMP, gotCP, mp, cp)
		}
		if !player.Revive() {
			t.Fatalf("round %d: Revive() = false", round)
		}
	}
}

// TestDeadNPCRejectsQueuedMPWriters reproduces a regeneration tick that saw
// a living NPC, then lost the race to a kill from another queue before it
// wrote MP: the write is dropped, as are every later MP writer and tick. A
// living NPC still spends and regenerates MP.
func TestDeadNPCRejectsQueuedMPWriters(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	hostile := spawnManaNPC(t, srv)

	half := hostile.MaxMPValue() / 2
	onQueue(t, hostile.Queue(), func() {
		if got := hostile.ReduceMP(half); got != half {
			t.Errorf("live ReduceMP(%v) = %v", half, got)
		}
		hostile.TickRegen()
	})
	if got := hostile.MPValue(); got <= hostile.MaxMPValue()-half {
		t.Fatalf("live NPC MP after regen = %v, want above %v", got, hostile.MaxMPValue()-half)
	}

	var mp float64
	onQueue(t, hostile.Queue(), func() {
		if hostile.AlikeDead() {
			t.Error("live NPC reports dead before the regen write")
			return
		}
		if !killNPC(t, hostile) {
			return
		}
		mp = hostile.MPValue()
		if got := hostile.AddMP(hostile.MPRegenRate() + 1); got != 0 {
			t.Errorf("AddMP after a concurrent kill = %v, want 0", got)
		}
		if got := hostile.ReduceMP(1); got != 0 {
			t.Errorf("ReduceMP after a concurrent kill = %v, want 0", got)
		}
		hostile.SetCurrentMP(int(hostile.MaxMPValue()))
	})
	if got := hostile.MPValue(); got != mp {
		t.Fatalf("corpse MP after stale writes = %v, want %v at death", got, mp)
	}

	onQueue(t, hostile.Queue(), hostile.TickRegen)
	if got := hostile.MPValue(); got != mp {
		t.Fatalf("corpse MP after a later regen tick = %v, want %v at death", got, mp)
	}
}

// TestConcurrentNPCMPWritersCannotChangeCorpse races MP writers against a
// kill from another goroutine; once the killing blow zeroes HP, no writer
// may move the corpse's MP.
func TestConcurrentNPCMPWritersCannotChangeCorpse(t *testing.T) {
	t.Parallel()
	const rounds, writers = 10, 4
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))

	for round := range rounds {
		hostile := spawnManaNPC(t, srv)
		hostile.ReduceMP(hostile.MaxMPValue() / 2)
		var stop atomic.Bool
		var wg sync.WaitGroup
		for i := range writers {
			wg.Go(func() {
				for !stop.Load() {
					if i%2 == 0 {
						hostile.AddMP(1)
					} else {
						hostile.ReduceMP(1)
					}
				}
			})
		}
		if !killNPC(t, hostile) {
			t.FailNow()
		}
		mp := hostile.MPValue()
		for range 1000 {
			hostile.AddMP(1)
			hostile.ReduceMP(1)
		}
		stop.Store(true)
		wg.Wait()
		if got := hostile.MPValue(); got != mp {
			t.Fatalf("round %d: corpse MP = %v after concurrent writers, want %v at death", round, got, mp)
		}
	}
}

// spawnManaNPC spawns a parked monster with an MP pool for the MP writers to
// move.
func spawnManaNPC(t *testing.T, srv *gameservertest.Server) *npc.Hostile {
	t.Helper()
	return srv.SpawnHostileNPCTemplateAt(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1,
		HPMax: 1000, MPMax: 1000, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20,
	}, location.Location{X: 60, Y: 20, Z: 30})
}

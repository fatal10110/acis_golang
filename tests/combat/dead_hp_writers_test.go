package combat

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	playermodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// hpWriterPlayer is the slice of a live player the dead-HP-writer tests drive.
type hpWriterPlayer interface {
	Dead() bool
	Die(attackable.Combatant) bool
	Revive(float64) bool
	CurrentHP() int
	CurrentCP() int
	SetCP(float64)
	Level() int
	AddLevel(*playermodel.LevelTable, *playermodel.Template, int) bool
	AddHP(float64) float64
	SetHP(float64)
	Queue() *sim.Queue
}

func bootHPWriterPlayer(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, hpWriterPlayer) {
	t.Helper()
	opts = append([]gameservertest.Option{gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1)}, opts...)
	srv := gameservertest.Boot(t, opts...)
	startInWorld(t, srv.Client)
	obj, ok := srv.State.Player(srv.SoleObjectID(t))
	if !ok {
		t.Fatal("player missing from world")
	}
	return srv, obj.(hpWriterPlayer)
}

// onQueue runs fn as a task on q and waits for it.
func onQueue(t *testing.T, q *sim.Queue, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !q.Post(func() { fn(); close(done) }) {
		t.Fatal("post: queue closed")
	}
	<-done
}

// TestDeadPlayerRejectsHPWriters pins that a corpse's HP stays at zero: a
// heal or set that passed its liveness check before the player died is
// dropped, while live healing and resurrection keep working.
func TestDeadPlayerRejectsHPWriters(t *testing.T) {
	t.Parallel()
	_, player := bootHPWriterPlayer(t)

	player.SetHP(10)
	if got := player.AddHP(5); got != 5 || player.CurrentHP() != 15 {
		t.Fatalf("live AddHP(5) = %v leaving HP %d, want 5 leaving 15", got, player.CurrentHP())
	}

	if player.Dead() {
		t.Fatal("fixture player starts dead")
	}
	// The liveness check above is now stale: the player dies before the writes.
	if !player.Die(nil) {
		t.Fatal("Die() = false for a living player")
	}
	if got := player.AddHP(100); got != 0 {
		t.Fatalf("dead AddHP(100) = %v, want 0", got)
	}
	player.SetHP(50)
	if got := player.CurrentHP(); got != 0 {
		t.Fatalf("dead player HP = %d after AddHP/SetHP, want 0", got)
	}

	if !player.Revive(0.5) || player.CurrentHP() <= 0 {
		t.Fatalf("Revive(0.5) left HP %d, want positive", player.CurrentHP())
	}
	player.SetHP(20)
	if got := player.AddHP(1); got != 1 || player.CurrentHP() != 21 {
		t.Fatalf("revived AddHP(1) = %v leaving HP %d, want 1 leaving 21", got, player.CurrentHP())
	}
}

// TestDeadPlayerLevelUpKeepsCorpseResources pins that a level gained while
// dead, as from a kill reward whose liveness check ran before the death,
// leaves the corpse's HP and CP where death left them, while a living
// player's level-up still refills them.
func TestDeadPlayerLevelUpKeepsCorpseResources(t *testing.T) {
	t.Parallel()
	levels := map[int]playermodel.Level{}
	for lvl := 1; lvl <= 10; lvl++ {
		levels[lvl] = playermodel.Level{RequiredExpToLevelUp: int64(lvl - 1)}
	}
	table, err := playermodel.NewLevelTable(levels)
	if err != nil {
		t.Fatalf("build level table: %v", err)
	}
	// Resource tables long enough that every level-up here refills.
	res := make([]float64, 10)
	for i := range res {
		res[i] = 100
	}
	tmpl := &playermodel.Template{HPTable: res, MPTable: res, CPTable: res}
	_, player := bootHPWriterPlayer(t, gameservertest.WithLevels(table))

	player.SetHP(1)
	player.SetCP(1)
	if !player.AddLevel(table, tmpl, 1) {
		t.Fatal("AddLevel(+1) did not level the living player")
	}
	if hp, cp := player.CurrentHP(), player.CurrentCP(); hp <= 1 || cp <= 1 {
		t.Fatalf("living player HP/CP after level-up = %d/%d, want refilled above 1/1", hp, cp)
	}

	player.SetCP(1)
	if !player.Die(nil) {
		t.Fatal("Die() = false for a living player")
	}
	before := player.Level()
	if !player.AddLevel(table, tmpl, 1) || player.Level() != before+1 {
		t.Fatalf("AddLevel did not level the dead player: level %d -> %d", before, player.Level())
	}
	if hp, cp := player.CurrentHP(), player.CurrentCP(); hp != 0 || cp != 1 {
		t.Fatalf("dead player HP/CP after level-up = %d/%d, want 0/1", hp, cp)
	}
	if !player.Dead() {
		t.Fatal("level-up revived the dead player")
	}
}

// TestConcurrentPlayerHPWritersCannotRaiseCorpse races heals and sets from
// other goroutines against a death on the player's own queue; once the death
// lands, no writer may lift the corpse's HP off zero.
func TestConcurrentPlayerHPWritersCannotRaiseCorpse(t *testing.T) {
	t.Parallel()
	const rounds, writers = 20, 4
	_, player := bootHPWriterPlayer(t)

	for round := range rounds {
		var stop atomic.Bool
		var wg sync.WaitGroup
		for range writers {
			wg.Go(func() {
				for !stop.Load() {
					player.AddHP(1)
					player.SetHP(20)
				}
			})
		}
		onQueue(t, player.Queue(), func() {
			if !player.Die(nil) {
				t.Errorf("round %d: Die() = false for a living player", round)
			}
		})
		// Writers keep running past the death before they stop.
		for range 1000 {
			player.AddHP(1)
		}
		stop.Store(true)
		wg.Wait()
		if got := player.CurrentHP(); got != 0 {
			t.Fatalf("round %d: corpse HP = %d after concurrent writers, want 0", round, got)
		}
		if !player.Revive(1) {
			t.Fatalf("round %d: Revive(1) = false", round)
		}
	}
}

// killNPC runs a lethal HP cost on hostile from a goroutine other than its
// own queue, the way a killing hit lands from the attacker's queue. It
// reports failure with Errorf rather than Fatal because callers may run it
// on a queue worker, where FailNow would strand the waiting test.
func killNPC(t *testing.T, hostile *npc.Hostile) bool {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		hostile.ConsumeHP(hostile.MaxHPValue() * 2)
	}()
	<-done
	if !hostile.Dead() {
		t.Error("lethal HP cost left the NPC alive")
		return false
	}
	return true
}

// TestDeadNPCRejectsQueuedRegen reproduces a regeneration tick that saw a
// living NPC, then lost the race to a kill from another queue before it
// wrote HP: the write is dropped and the corpse stays at zero, as does every
// later tick. A living NPC still regenerates.
func TestDeadNPCRejectsQueuedRegen(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	hostile := srv.SpawnHostileNPC(t)

	half := int(hostile.MaxHPValue()) / 2
	onQueue(t, hostile.Queue(), func() {
		hostile.SetHP(float64(half))
		hostile.TickRegen()
	})
	if got := hostile.CurrentHP(); got <= half {
		t.Fatalf("live NPC HP after regen = %d, want above %d", got, half)
	}

	onQueue(t, hostile.Queue(), func() {
		if hostile.AlikeDead() {
			t.Error("live NPC reports dead before the regen write")
			return
		}
		if !killNPC(t, hostile) {
			return
		}
		if got := hostile.AddHP(hostile.HPRegenRate() + 1); got != 0 {
			t.Errorf("AddHP after a concurrent kill = %v, want 0", got)
		}
	})
	if got := hostile.CurrentHP(); got != 0 {
		t.Fatalf("corpse HP after stale regen write = %d, want 0", got)
	}

	onQueue(t, hostile.Queue(), hostile.TickRegen)
	if got := hostile.CurrentHP(); got != 0 {
		t.Fatalf("corpse HP after a later regen tick = %d, want 0", got)
	}
}

// TestConcurrentNPCRegenCannotRaiseCorpse races heal writers against a kill
// from another goroutine; once the killing blow zeroes HP, no writer may lift
// the corpse's HP off zero.
func TestConcurrentNPCRegenCannotRaiseCorpse(t *testing.T) {
	t.Parallel()
	const rounds, writers = 10, 4
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))

	for round := range rounds {
		hostile := srv.SpawnHostileNPC(t)
		var stop atomic.Bool
		var wg sync.WaitGroup
		for range writers {
			wg.Go(func() {
				for !stop.Load() {
					hostile.AddHP(1)
				}
			})
		}
		if !killNPC(t, hostile) {
			t.FailNow()
		}
		for range 1000 {
			hostile.AddHP(1)
		}
		stop.Store(true)
		wg.Wait()
		if got := hostile.CurrentHP(); got != 0 {
			t.Fatalf("round %d: corpse HP = %d after concurrent writers, want 0", round, got)
		}
	}
}

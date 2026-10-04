package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// regenPhasePlayer is a player in the world on a driven clock, with the
// production regeneration sweep.
type regenPhasePlayer struct {
	srv   *gameservertest.Server
	c     *player.Character
	objID int32
	regen *task.NPCRegen
}

// regenPhaseDrop is the HP, MP and CP drop the scenarios start from.
var regenPhaseDrop = player.Resources{MaxHP: 100, CurrentHP: 10, MaxMP: 100, CurrentMP: 10, MaxCP: 100, CurrentCP: 10}

// bootRegenPhasePlayer enters the world with a player at full HP, MP and CP,
// its regeneration phase idle, so the scenario's drop starts the phase.
func bootRegenPhasePlayer(t *testing.T) *regenPhasePlayer {
	t.Helper()
	// The class template carries a row for every level up to the one the
	// level-up scenario reaches, so the level-up refills.
	tmpl := gameservertest.ClassTemplate()
	tmpl.HPTable = []float64{80, 80, 80, 80, 80, 90}
	tmpl.MPTable = []float64{30, 30, 30, 30, 30, 35}
	tmpl.CPTable = []float64{32, 32, 32, 32, 32, 36}
	srv, c, objID := bootInZones(t, zone.NewIndex(),
		gameservertest.WithLevels(regenPhaseLevels(t)), gameservertest.WithClassTemplate(tmpl))
	if !srv.DrivesClock() {
		t.Skip("pinning the regeneration phase needs the driven clock")
	}
	c.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 100, CurrentMP: 100, MaxCP: 100, CurrentCP: 100})
	full := c.ResourceValues()
	c.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: full.MaxHP, MaxMP: 100, CurrentMP: full.MaxMP, MaxCP: 100, CurrentCP: full.MaxCP})
	if c.Regen().Active() {
		t.Fatal("regeneration armed at full HP, MP and CP")
	}
	return &regenPhasePlayer{srv: srv, c: c, objID: objID, regen: task.NewNPCRegen(srv.State)}
}

// regenPhaseLevels is a level table whose next threshold sits low enough
// that a small reward promotes the level-5 fixture exactly once.
func regenPhaseLevels(t *testing.T) *player.LevelTable {
	t.Helper()
	table, err := player.NewLevelTable(map[int]player.Level{
		1: {RequiredExpToLevelUp: 0},
		2: {RequiredExpToLevelUp: 1},
		3: {RequiredExpToLevelUp: 2},
		4: {RequiredExpToLevelUp: 3},
		5: {RequiredExpToLevelUp: 4},
		6: {RequiredExpToLevelUp: 5},
		7: {RequiredExpToLevelUp: 1_000_000_000},
	})
	if err != nil {
		t.Fatalf("build level table: %v", err)
	}
	return table
}

// after advances the clock by d, runs one sweep and returns the player's
// resources.
func (p *regenPhasePlayer) after(t *testing.T, d time.Duration) player.Resources {
	t.Helper()
	p.srv.Advance(t, d)
	p.regen.Tick()
	p.srv.Settle(t)
	return p.c.ResourceValues()
}

// TestPlayerRegenPhaseSurvivesTeleport pins that a teleport leaves the
// regeneration task on its grid (Creature.teleportTo and onTeleported never
// stop it; startHpMpRegeneration checks only death): a tick that lands while
// the player is off the grid awaiting Appearing keeps the phase armed, and
// the next tick lands exactly one period later, after Appearing.
func TestPlayerRegenPhaseSurvivesTeleport(t *testing.T) {
	t.Parallel()
	p := bootRegenPhasePlayer(t)
	p.c.SetResourceValues(regenPhaseDrop)
	dropAt := p.c.Queue().Now()
	if got := p.after(t, time.Second); got.CurrentHP != 10 {
		t.Fatalf("HP 1s after the drop = %v, want 10", got.CurrentHP)
	}

	x, y, z := p.srv.PlayerPosition(t, p.objID)
	p.c.TeleportTo(x+300, y, z, 0)
	if p.c.Visible() {
		t.Fatal("player still on the grid before Appearing")
	}
	first := p.after(t, 2*time.Second) // 3s: the tick lands off the grid
	if first.CurrentHP <= 10 {
		t.Fatalf("HP 3s after the drop, mid-teleport = %v, want regenerated", first.CurrentHP)
	}
	if !p.c.Regen().Active() {
		t.Fatal("a tick mid-teleport disarmed the regeneration of a player still short")
	}

	readUntilQuiet(p.srv.Client)
	appear(t, p.srv.Client)
	if !p.c.Regen().Active() {
		t.Fatal("regeneration idle after Appearing")
	}
	// Reading the client moved the driven clock; the next tick is due on
	// the drop's grid, two periods after it.
	left := dropAt.Add(2 * task.NPCRegenTick).Sub(p.c.Queue().Now())
	if left <= time.Millisecond {
		t.Fatalf("Appearing took the clock to %v before the second tick", 2*task.NPCRegenTick-left)
	}
	if got := p.after(t, left-time.Millisecond); got.CurrentHP != first.CurrentHP {
		t.Fatalf("HP 5.999s after the drop = %v, want %v", got.CurrentHP, first.CurrentHP)
	}
	if got := p.after(t, time.Millisecond); got.CurrentHP <= first.CurrentHP {
		t.Fatalf("HP 6s after the drop = %v, want above %v: the teleport lost the phase", got.CurrentHP, first.CurrentHP)
	}
}

// TestPlayerLevelUpRefillRestartsRegenPhase pins the level-up refill's stop
// of the regeneration task (PlayableStatus.addLevel's setMaxHpMp and
// PlayerStatus.addLevel's setCp): a player hit, refilled by a level-up
// before the first tick and hit again takes no tick on the first hit's
// grid, only one period after the second hit.
func TestPlayerLevelUpRefillRestartsRegenPhase(t *testing.T) {
	t.Parallel()
	p := bootRegenPhasePlayer(t)
	p.c.SetResourceValues(regenPhaseDrop)
	if got := p.after(t, 2500*time.Millisecond); got.CurrentHP != 10 {
		t.Fatalf("HP 2.5s after the first hit = %v, want 10", got.CurrentHP)
	}

	level := p.c.CharLevel
	p.c.AddExpAndSp(5, 0)
	full := p.c.ResourceValues()
	if p.c.CharLevel <= level || full.CurrentHP != full.MaxHP || full.CurrentMP != full.MaxMP || full.CurrentCP != full.MaxCP {
		t.Fatalf("after the level-up: level %d resources %+v, want above %d and full", p.c.CharLevel, full, level)
	}
	if p.c.Regen().Active() {
		t.Fatal("regeneration still armed after the level-up refill")
	}

	p.srv.Advance(t, 400*time.Millisecond)
	p.c.ReduceCurrentHP(20) // second hit at 2.9s
	hit := full.CurrentHP - 20
	if got := p.after(t, 100*time.Millisecond); got.CurrentHP != hit {
		t.Fatalf("HP at the first hit's 3s mark = %v, want %v: ticked on the stale phase", got.CurrentHP, hit)
	}
	if got := p.after(t, task.NPCRegenTick-100*time.Millisecond-time.Millisecond); got.CurrentHP != hit {
		t.Fatalf("HP 2.999s after the second hit = %v, want %v", got.CurrentHP, hit)
	}
	if got := p.after(t, time.Millisecond); got.CurrentHP <= hit {
		t.Fatalf("HP 3s after the second hit = %v, want above %v", got.CurrentHP, hit)
	}
}

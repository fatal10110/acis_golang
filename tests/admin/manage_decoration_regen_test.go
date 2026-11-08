package admin

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// regenTreeHPRegen is the base HP regeneration of the regenerating tree
// fixture: a tenth of its 1000 HP per tick.
const regenTreeHPRegen = 100

// regenTreeTemplates are killTemplates with the tree regenerating
// regenTreeHPRegen HP per tick.
func regenTreeTemplates(undying bool) *npc.Table {
	return npc.NewTable([]*npc.Template{{
		ID: killTreeID, TemplateID: killTreeID, Type: "ChristmasTree", Name: "Christmas Tree", Level: 1,
		HPMax: 1000, CorpseTime: reviveCorpseTime, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true,
		Undying: undying, HPRegen: regenTreeHPRegen,
	}})
}

// bootRegenTreeAdmin is bootTreeAdmin on the driven clock, with the
// production regeneration sweep the test runs itself.
func bootRegenTreeAdmin(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, *npc.Decoration, *task.NPCRegen) {
	t.Helper()
	srv, _, _, tree := bootTreeAdmin(t, opts...)
	if !srv.DrivesClock() {
		t.Skip("pinning the regeneration grid needs the driven clock")
	}
	return srv, tree, task.NewNPCRegen(srv.State)
}

// hpAfter advances the clock by d, runs one sweep and returns tree's HP.
func hpAfter(t *testing.T, srv *gameservertest.Server, sweep *task.NPCRegen, tree *npc.Decoration, d time.Duration) float64 {
	t.Helper()
	srv.Advance(t, d)
	sweep.Tick()
	srv.Settle(t)
	return tree.HP()
}

// requireTreeRegen runs the sweep one regeneration period at a time from a
// tree at from HP, checking that each tick, and nothing earlier, adds the
// tree's HP regeneration rate up to its maximum, that the phase then stops,
// and that no tick shows anybody a StatusUpdate: nobody selects a tree.
func requireTreeRegen(t *testing.T, srv *gameservertest.Server, sweep *task.NPCRegen, tree *npc.Decoration, from float64) {
	t.Helper()
	rate, maxHP := tree.HPRegenRate(), tree.MaxHPValue()
	if tree.HP() != from || !tree.Regen().Active() {
		t.Fatalf("tree before regeneration: HP %v armed %v, want %v of %v HP with its regeneration armed", tree.HP(), tree.Regen().Active(), from, maxHP)
	}
	drain(t, srv.Client)
	if got := hpAfter(t, srv, sweep, tree, creature.RegenPeriod-time.Millisecond); got != from {
		t.Fatalf("tree HP %v just before its first tick, want %v: regenerated early", got, from)
	}
	for want, d := min(from+rate, maxHP), time.Millisecond; ; want, d = min(want+rate, maxHP), creature.RegenPeriod {
		if got := hpAfter(t, srv, sweep, tree, d); got != want {
			t.Fatalf("tree HP %v after a tick, want %v (rate %v)", got, want, rate)
		}
		if want == maxHP {
			break
		}
	}
	if tree.Regen().Active() {
		t.Fatal("full tree still has its regeneration armed")
	}
	frames := settle(t, srv.Client)
	if n := countFramesFor(frames, serverpackets.OpcodeStatusUpdate, tree.ObjectID()); n != 0 {
		t.Fatalf("regeneration frames = %x, want no StatusUpdate of the tree", testsupport.FrameOpcodes(frames))
	}
	if got := hpAfter(t, srv, sweep, tree, 2*creature.RegenPeriod); got != maxHP {
		t.Fatalf("full tree HP %v after more periods, want its maximum %v", got, maxHP)
	}
}

// TestAdminManageDecorationRegenAfterRes pins a Christmas tree revived by
// //res below its maximum HP: like any creature set short of its HP, it
// regenerates on the fixed three-second grid, its first tick one period
// after the revive, each tick adding its HP regeneration rate (its base
// regeneration times the configured multiplier, through its stats) until it
// is full.
func TestAdminManageDecorationRegenAfterRes(t *testing.T) {
	t.Parallel()
	// The subtests run in order so the second compares against the shipped
	// rate the first reads.
	var rates [2]float64
	for i, opts := range [][]gameservertest.Option{nil, {gameservertest.WithRegenMultipliers(2, 1, 1)}} {
		t.Run([]string{"shipped", "multiplied"}[i], func(t *testing.T) {
			opts = append([]gameservertest.Option{gameservertest.WithNPCs(regenTreeTemplates(false))}, opts...)
			srv, tree, sweep := bootRegenTreeAdmin(t, opts...)
			rates[i] = tree.HPRegenRate()
			manageAround(t, srv, tree, "kill")
			if !tree.Dead() || tree.Regen().Active() {
				t.Fatalf("killed tree: dead %v regen armed %v, want dead with no regeneration", tree.Dead(), tree.Regen().Active())
			}
			manageAround(t, srv, tree, "res")
			requireTreeRegen(t, srv, sweep, tree, revivedHP(tree.MaxHPValue()))
		})
	}
	if t.Failed() {
		return
	}
	if rates[0] <= 0 || math.Abs(rates[1]-2*rates[0]) > 1e-9 {
		t.Fatalf("tree HP regen rates shipped %v, at HpRegenMultiplier 2 %v; want a positive rate, doubled", rates[0], rates[1])
	}
}

// TestAdminManageUndyingDecorationRegen pins //kill over an undying tree,
// as the shipped trees are: the hit leaves it alive at one HP, which starts
// its regeneration, and it climbs back to its maximum HP.
func TestAdminManageUndyingDecorationRegen(t *testing.T) {
	t.Parallel()
	srv, tree, sweep := bootRegenTreeAdmin(t, gameservertest.WithNPCs(regenTreeTemplates(true)))
	manageAround(t, srv, tree, "kill")
	requireTreeRegen(t, srv, sweep, tree, 1)
}

// TestAdminManageDecorationRegenStopsAtDeath pins a revived tree killed
// again before its first regeneration tick: death ends the regeneration, so
// its corpse stays at zero HP.
func TestAdminManageDecorationRegenStopsAtDeath(t *testing.T) {
	t.Parallel()
	srv, tree, sweep := bootRegenTreeAdmin(t, gameservertest.WithNPCs(regenTreeTemplates(false)))
	manageAround(t, srv, tree, "kill")
	manageAround(t, srv, tree, "res")
	if !tree.Regen().Active() {
		t.Fatal("revived tree has no regeneration armed")
	}
	manageAround(t, srv, tree, "kill")
	if !tree.Dead() || tree.Regen().Active() {
		t.Fatalf("tree killed again: dead %v regen armed %v, want dead with no regeneration", tree.Dead(), tree.Regen().Active())
	}
	if hp := hpAfter(t, srv, sweep, tree, 2*creature.RegenPeriod); hp != 0 || !tree.Dead() {
		t.Fatalf("corpse after two periods: HP %v dead %v, want dead at 0 HP", hp, tree.Dead())
	}
}

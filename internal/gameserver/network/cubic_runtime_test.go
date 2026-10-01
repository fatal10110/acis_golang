package network

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func cubicRuntimeDef(id cubic.ID) modelskill.Definition {
	return modelskill.Definition{
		Level: 1, SkillType: "SUMMON", IsCubic: true, NpcID: int(id),
		CubicActivationTime: 5, SummonTotalLifeTime: 900_000,
	}
}

func liveCubicRuntime(live *livePlayer, id cubic.ID) *cubic.Runtime {
	live.cubicsMu.Lock()
	defer live.cubicsMu.Unlock()
	return live.cubics[id]
}

// A party member's mass cubic can evict a cubic from live's list on the
// caster's queue before live's own cast syncs that cubic's runtime. The
// late sync must not start a runtime for a cubic the list no longer holds.
func TestSyncCubicRuntimeSkipsACubicEvictedBeforeTheSync(t *testing.T) {
	link := &GameClientLink{log: zerolog.Nop()}
	live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})

	live.Character.AddOrRefreshCubic(cubic.Life, false)
	live.Character.AddOrRefreshCubic(cubic.Storm, true) // no mastery: evicts Life

	link.syncCubicRuntime(live, cubic.Life, cubicRuntimeDef(cubic.Life))
	if r := liveCubicRuntime(live, cubic.Life); r != nil {
		t.Fatal("late sync started a runtime for the evicted Life Cubic")
	}
	link.syncCubicRuntime(live, cubic.Storm, cubicRuntimeDef(cubic.Storm))
	if r := liveCubicRuntime(live, cubic.Storm); r == nil {
		t.Fatal("Storm Cubic, still listed, has no runtime")
	}
}

// An evicted cubic's runtime stays dead even when the same id is granted
// again: its pending cast effect does not land and its late disappear timer
// leaves the new cubic of that id alone.
func TestEvictedCubicRuntimeDoesNotActForARegrantedID(t *testing.T) {
	link := &GameClientLink{log: zerolog.Nop()}
	live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})

	live.Character.AddOrRefreshCubic(cubic.Life, false)
	link.syncCubicRuntime(live, cubic.Life, cubicRuntimeDef(cubic.Life))
	evicted := liveCubicRuntime(live, cubic.Life)

	live.Character.AddOrRefreshCubic(cubic.Storm, false)
	link.syncCubicRuntime(live, cubic.Storm, cubicRuntimeDef(cubic.Storm))
	live.Character.AddOrRefreshCubic(cubic.Life, false)
	link.syncCubicRuntime(live, cubic.Life, cubicRuntimeDef(cubic.Life))
	regranted := liveCubicRuntime(live, cubic.Life)

	if regranted == nil || regranted == evicted {
		t.Fatalf("regranted Life runtime = %p, want a fresh runtime (evicted %p)", regranted, evicted)
	}
	if live.cubicStillActive(cubic.Life, evicted) {
		t.Fatal("evicted Life runtime still reads as active after the id was regranted")
	}
	if !live.cubicStillActive(cubic.Life, regranted) {
		t.Fatal("regranted Life runtime does not read as active")
	}

	link.expireCubic(live, cubic.Life, evicted)
	if got := live.Character.CubicIDs(); len(got) != 1 || got[0] != int(cubic.Life) {
		t.Fatalf("CubicIDs after the evicted runtime's late expiry = %v, want [%d]", got, cubic.Life)
	}
	if liveCubicRuntime(live, cubic.Life) != regranted {
		t.Fatal("evicted runtime's late expiry dropped the regranted runtime")
	}
}

// A combat-stance entry can snapshot a cubic's runtime, lose the race to an
// eviction that stops it, and restart its tick afterwards. That stale tick
// must neither act nor broadcast a MagicSkillUse for a cubic the client no
// longer shows.
func TestStaleCubicTickDoesNotFire(t *testing.T) {
	const healSkill = 4051
	link := &GameClientLink{
		log:    zerolog.Nop(),
		skills: skillstate.NewPersistence(nil, skillTable(modelskill.Definition{ID: healSkill, Level: 1, Power: 10})),
	}
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	live.Character.SetRollSource(func(int) int { return 0 })
	live.Character.SetResourceValues(player.Resources{MaxHP: 80, CurrentHP: 20, MaxMP: 30, CurrentMP: 30})

	live.Character.AddOrRefreshCubic(cubic.Life, false)
	link.syncCubicRuntime(live, cubic.Life, cubicRuntimeDef(cubic.Life))
	evicted := liveCubicRuntime(live, cubic.Life)
	live.Character.AddOrRefreshCubic(cubic.Storm, false)
	link.syncCubicRuntime(live, cubic.Storm, cubicRuntimeDef(cubic.Storm))

	evicted.Action() // the late combat-stance restart
	link.fireCubic(live, cubic.Life, evicted)
	if got := frames.Frames(); len(got) != 0 {
		t.Fatalf("stale Life Cubic tick sent %d frames, want none", len(got))
	}
}

// cubicOwnerOnClock is a live player holding a granted Storm Cubic whose
// timers run on a driven clock.
func cubicOwnerOnClock(t *testing.T, link *GameClientLink, id int32, lifetime time.Duration) (*livePlayer, *sim.Inline) {
	t.Helper()
	in := sim.NewInline(time.Unix(0, 0))
	live := newTestLivePlayer(t, id, &testsupport.FrameCapture{})
	live.Character.Live.SetQueue(in.NewQueue("owner"))
	def := cubicRuntimeDef(cubic.Storm)
	def.SummonTotalLifeTime = int(lifetime / time.Millisecond)
	live.Character.AddOrRefreshCubic(cubic.Storm, false)
	link.syncCubicRuntime(live, cubic.Storm, def)
	return live, in
}

// A teleport halts the player through Stop, which must leave its cubics
// alone: the cubic still expires at its granted lifetime. Detach is the one
// path that stops them: no timer of a logged-out player's cubic runs.
func TestCubicsStopOnDetachButNotOnTeleportHalt(t *testing.T) {
	const lifetime = 10 * time.Second
	link := &GameClientLink{log: zerolog.Nop()}

	halted, in := cubicOwnerOnClock(t, link, 1, lifetime)
	sim.RunOwned(halted.Queue(), halted.Stop)
	in.Advance(lifetime + time.Second)
	if got := halted.Character.CubicIDs(); len(got) != 0 {
		t.Fatalf("CubicIDs after the lifetime of a teleport-halted owner = %v, want none (the disappear timer kept running)", got)
	}

	detached, in := cubicOwnerOnClock(t, link, 2, lifetime)
	runtime := liveCubicRuntime(detached, cubic.Storm)
	runtime.Action()
	sim.RunOwned(detached.Queue(), func() { link.detachLivePlayer(detached) })
	in.Advance(lifetime + time.Second)
	if got := detached.Character.CubicIDs(); len(got) != 1 || got[0] != int(cubic.Storm) {
		t.Fatalf("CubicIDs after detach and the lifetime = %v, want [%d]: no cubic timer runs after detach", got, cubic.Storm)
	}
}

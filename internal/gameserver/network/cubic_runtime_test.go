package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func cubicRuntimeDef(id cubic.ID) modelskill.Definition {
	return modelskill.Definition{
		SkillType: "SUMMON", IsCubic: true, NpcID: int(id),
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

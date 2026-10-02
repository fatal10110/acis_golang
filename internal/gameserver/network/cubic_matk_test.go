package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// A cubic's M.Atk is its granting skill's power truncated to a whole
// number (CubicList.addOrRefreshCubic's (int) matk), fixed for its life: a
// refresh by a stronger grant keeps it, as it keeps the level.
func TestCubicMAtkIsTheGrantingPowerKeptAcrossRefresh(t *testing.T) {
	link := &GameClientLink{log: zerolog.Nop()}
	live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})

	def := cubicRuntimeDef(cubic.Storm)
	def.Power = 919.9
	live.Character.AddOrRefreshCubic(cubic.Storm, false)
	link.syncCubicRuntime(live, cubic.Storm, def)
	runtime := liveCubicRuntime(live, cubic.Storm)
	if runtime == nil || runtime.MAtk != 919 {
		t.Fatalf("Storm Cubic runtime = %v, want M.Atk 919", runtime)
	}

	stronger := def
	stronger.Power = 1975
	live.Character.AddOrRefreshCubic(cubic.Storm, false)
	link.syncCubicRuntime(live, cubic.Storm, stronger)
	if got := liveCubicRuntime(live, cubic.Storm); got != runtime || got.MAtk != 919 {
		t.Fatalf("refreshed Storm Cubic runtime M.Atk = %d (same runtime %v), want 919 kept", got.MAtk, got == runtime)
	}
}

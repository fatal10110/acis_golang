package admin

import (
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: //reload config (AdminReload "config") reruns Config.loadGameServer,
// which rewrites the Config statics; each setting below is read from them
// at every use, so the reloaded value reaches what is already in the world:
// RATE_QUEST_* in Quest's drop and reward helpers, MAX_BUFFS_AMOUNT in
// Creature.getMaxBuffCount, MAX_DRIFT_RANGE in Spawn.isInMyTerritory and
// Attackable.getDriftRange, SHOW_NPC_LVL in AbstractNpcInfo.NpcInfo and
// PVP_NORMAL_TIME/PVP_PVP_TIME in Player.updatePvPStatus.

// liveSettings is the PlayerConfig base each live reload test reloads, the
// boot settings bootAdmin's characters already play by.
func liveSettings() network.PlayerConfig {
	return network.PlayerConfig{RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1}
}

// flipReloads is a //reload config hook that serves on, then off, then on
// again, and so on, counting the reloads it served in calls.
func flipReloads(calls *atomic.Int32, off, on network.PlayerConfig) gameservertest.Option {
	return gameservertest.WithDataReloads(network.DataReloads{Config: func() (network.PlayerConfig, error) {
		if calls.Add(1)%2 == 1 {
			return on, nil
		}
		return off, nil
	}})
}

// reloadsInBackground has c send rounds //reload config commands from its
// own goroutine, without reading the answers, and reports the first send
// error, or nil, once they are all sent.
func reloadsInBackground(c *testsupport.ScriptedClient, rounds int) <-chan error {
	sent := make(chan error, 1)
	go func() {
		for range rounds {
			if err := c.TrySend(encodeBuildCmd("reload config")); err != nil {
				sent <- err
				return
			}
		}
		sent <- nil
	}()
	return sent
}

// awaitReloads waits until the hook served want reloads and the background
// sender reported.
func awaitReloads(t *testing.T, srv *gameservertest.Server, calls *atomic.Int32, want int32, sent <-chan error) {
	t.Helper()
	if err := <-sent; err != nil {
		t.Fatalf("send reload: %v", err)
	}
	srv.AdvanceUntil(t, "every reload served", func() bool { return calls.Load() >= want })
}

package admin

import (
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Two players.properties a reload swaps between: the boot one, and one with
// more buff slots and fewer inventory slots.
var (
	bootRules     = network.PlayerConfig{RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1}
	reloadedRules = network.PlayerConfig{
		RespawnRestoreHP: 0.7, MaxBuffsAmount: 24, WeightLimitMultiplier: 1,
		InventorySlots: player.InventorySlots{NoDwarf: 50, Dwarf: 60, Configured: true},
	}
)

// TestAdminReloadConfigRulesReachOnline: //reload config puts the reloaded
// character rules in force for a player already in the world, as the
// reference reads players.properties on each use: its buff and inventory
// slots follow the reload at once.
func TestAdminReloadConfigRulesReachOnline(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithDataReloads(fixedConfig(reloadedRules)))
	gm := srv.Client
	enterWorld(t, gm)
	c := onlineCharacter(t, srv, gmID)
	bootLimit := c.InventoryLimit()
	if got := c.MaxBuffCount(); got != 20 {
		t.Fatalf("buff slots before the reload = %d, want 20", got)
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload config")), configReloaded)
	if got := c.MaxBuffCount(); got != 24 {
		t.Fatalf("buff slots after the reload = %d, want 24", got)
	}
	if got := c.InventoryLimit(); got != 50 || got == bootLimit {
		t.Fatalf("inventory slots after the reload = %d (%d before), want 50", got, bootLimit)
	}
}

// TestAdminReloadConfigRulesRaceFree reloads the character rules back and
// forth while the player's rules are read: under -race nothing races, and
// each read sees the boot rules or the reloaded ones.
func TestAdminReloadConfigRulesRaceFree(t *testing.T) {
	t.Parallel()
	var flip atomic.Bool
	reloads := network.DataReloads{Config: func() (network.PlayerConfig, error) {
		if flip.Load() {
			return reloadedRules, nil
		}
		return bootRules, nil
	}}
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithDataReloads(reloads))
	gm := srv.Client
	enterWorld(t, gm)
	c := onlineCharacter(t, srv, gmID)
	bootLimit := c.InventoryLimit()

	const rounds = 20
	sent := make(chan error, 1)
	go func() {
		for i := range rounds {
			flip.Store(i%2 == 0)
			if err := gm.TrySend(encodeBuildCmd("reload config")); err != nil {
				sent <- err
				return
			}
		}
		sent <- nil
	}()
	for range 200 {
		if got := c.MaxBuffCount(); got != 20 && got != 24 {
			t.Fatalf("buff slots = %d, want 20 or 24", got)
		}
		if got := c.InventoryLimit(); got != bootLimit && got != 50 {
			t.Fatalf("inventory slots = %d, want %d or 50", got, bootLimit)
		}
	}
	if err := <-sent; err != nil {
		t.Fatalf("send reload: %v", err)
	}
	testsupport.SyncBarrierFrames(t, gm, func() { gm.Send(encodeManorBarrier()) }, serverpackets.OpcodeExtended)
	// The last reload read the boot rules.
	if got := c.MaxBuffCount(); got != 20 {
		t.Fatalf("buff slots after the last reload = %d, want 20", got)
	}
}

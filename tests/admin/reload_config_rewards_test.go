package admin

import (
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// rewardWeaponID is the shared catalog's non-stackable weapon.
const rewardWeaponID int32 = 30

// reloadRewardMonster is a level-1 monster paying 100 exp and 10 sp, with
// one guaranteed drop of three weapons.
func reloadRewardMonster(id int) *npc.Template {
	return &npc.Template{
		ID: id, TemplateID: id, Name: "Reward Dummy", Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		RewardExp: 100, RewardSp: 10,
		Drops: []item.DropCategory{{Kind: item.DropNormal, Chance: 100, Drops: []item.Drop{{ItemID: rewardWeaponID, Min: 3, Max: 3, Chance: 100}}}},
	}
}

// TestAdminReloadConfigKillRewardsTakeReloadedSettings: a kill after
// //reload config pays by the reloaded kill reward settings, as the
// reference reads its rates and MULTIPLE_ITEM_DROP on each kill. Before the
// reload the kill pays 100 exp and 10 sp and drops its three weapons as
// three; after it, at exp rate 3 and sp rate 2 with the multiple item drop
// rule off, the same kill pays 300 exp and 20 sp and drops one weapon.
func TestAdminReloadConfigKillRewardsTakeReloadedSettings(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithDataReloads(fixedConfig(network.PlayerConfig{
			RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1,
			KillRewards: &manager.KillRewardSettings{
				Rates:      item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1},
				PartyRange: 1500, RateXP: 3, RateSP: 2,
			},
		})))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)

	exp, sp, weapons := killRewardDummy(t, srv, gmID, 25001)
	if exp != 100 || sp != 10 || weapons != 3 {
		t.Fatalf("boot kill paid %d exp, %d sp and dropped %d weapons; want 100, 10 and 3", exp, sp, weapons)
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload config")), configReloaded)

	exp, sp, weapons = killRewardDummy(t, srv, gmID, 25002)
	if exp != 300 || sp != 20 || weapons != 1 {
		t.Fatalf("reloaded kill paid %d exp, %d sp and dropped %d weapons; want 300, 20 and 1", exp, sp, weapons)
	}
}

// TestAdminReloadConfigKillRewardsRaceFree: kills while //reload config
// swaps the kill reward settings back and forth each pay by one set whole,
// the boot one or the reloaded one, never its exp from one and its sp or
// drops from the other.
func TestAdminReloadConfigKillRewardsRaceFree(t *testing.T) {
	t.Parallel()
	boot := manager.KillRewardSettings{
		Rates:      item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1},
		PartyRange: 1500, RateXP: 1, RateSP: 1, MultipleItemDrop: true,
	}
	reloaded := boot
	reloaded.RateXP, reloaded.RateSP, reloaded.MultipleItemDrop = 3, 2, false
	var flip atomic.Bool
	reloads := network.DataReloads{Config: func() (network.PlayerConfig, error) {
		settings := network.PlayerConfig{RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1, KillRewards: &boot}
		if flip.Load() {
			settings.KillRewards = &reloaded
		}
		return settings, nil
	}}
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithDataReloads(reloads))
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)

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
	for i := range 6 {
		exp, sp, weapons := killRewardDummy(t, srv, gmID, 25010+i)
		if (exp != 100 || sp != 10 || weapons != 3) && (exp != 300 || sp != 20 || weapons != 1) {
			t.Fatalf("kill %d paid %d exp, %d sp and dropped %d weapons; want 100, 10, 3 or 300, 20, 1", i, exp, sp, weapons)
		}
	}
	if err := <-sent; err != nil {
		t.Fatalf("send reload: %v", err)
	}
}

// killRewardDummy spawns a reward dummy of npc id beside the player killer
// and kills it with the player's hit, returning the exp and sp the player
// was told it earned and how many weapons the kill dropped.
func killRewardDummy(t *testing.T, srv *gameservertest.Server, killerID int32, id int) (exp, sp int32, weapons int) {
	t.Helper()
	before := groundCount(srv, rewardWeaponID)
	x, y, z := srv.PlayerPosition(t, killerID)
	monster := srv.SpawnHostileNPCTemplateAt(t, reloadRewardMonster(id), location.Location{X: x + 50, Y: y, Z: z})
	srv.Settle(t)
	drain(t, srv.Client)
	if !monster.TakeDamage(1_000_000, onlineCharacter(t, srv, killerID)) {
		t.Fatal("lethal hit did not kill the dummy")
	}
	srv.Settle(t)
	earned := false
	for _, f := range srv.Client.ReadQueued() {
		if f[0] != serverpackets.OpcodeSystemMessage || int(wire.NewReader(f[1:]).ReadInt32()) != serverpackets.SystemMessageYouEarnedS1ExpAndS2SP {
			continue
		}
		if earned {
			t.Fatal("two exp/SP gain messages for one kill")
		}
		r := wire.NewReader(f[5:])
		r.ReadInt32() // param count
		r.ReadInt32() // number type
		exp = r.ReadInt32()
		r.ReadInt32()
		sp = r.ReadInt32()
		earned = true
	}
	if !earned {
		t.Fatal("no exp/SP gain message for the kill")
	}
	return exp, sp, groundCount(srv, rewardWeaponID) - before
}

// groundCount counts the ground items of templateID.
func groundCount(srv *gameservertest.Server, templateID int32) int {
	n := 0
	for _, g := range srv.GroundItems.Snapshots(nil) {
		if g.TemplateID == templateID {
			n++
		}
	}
	return n
}

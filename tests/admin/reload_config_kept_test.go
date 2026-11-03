package admin

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Shared catalog items a death may drop: a potion and a tunic.
const (
	keptPotionID int32 = 20
	keptTunicID  int32 = 40
)

// fixedConfig is a //reload config hook that reads settings every time.
func fixedConfig(settings network.PlayerConfig) network.DataReloads {
	return network.DataReloads{Config: func() (network.PlayerConfig, error) { return settings, nil }}
}

// TestAdminReloadConfigDeathDropTakesReloadedRules: a player in the world
// when //reload config runs dies under the reloaded death-drop rules, all of
// them, as the reference reads them on each death. The game master here
// entered with the potion kept; the reload keeps the tunic instead, so its
// karma death drops the potion and keeps the tunic.
func TestAdminReloadConfigDeathDropTakesReloadedRules(t *testing.T) {
	t.Parallel()
	every := player.DeathDropRates{Chance: 100, Equip: 100, EquipWeapon: 100, Item: 100, Limit: 10}
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithDeathDrop(player.DeathDropRules{Karma: every, GMDrops: true, Kept: []int32{keptPotionID}}),
		gameservertest.WithDataReloads(fixedConfig(network.PlayerConfig{
			RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1,
			DeathDrop: player.DeathDropRules{Karma: every, GMDrops: true, Kept: []int32{keptTunicID}},
		})))
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET karma = 100 WHERE obj_Id = ?", gmID); err != nil {
		t.Fatalf("set karma: %v", err)
	}
	srv.GiveItem(t, gmID, keptPotionID, 5)
	srv.GiveItem(t, gmID, keptTunicID, 1)
	gm := srv.Client
	enterWorld(t, gm)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload config")), configReloaded)

	var killer attackable.Combatant = srv.SpawnHostileNPC(t)
	victim := onlineCharacter(t, srv, gmID)
	done := make(chan struct{})
	if !srv.PlayerQueue(t, gmID).Post(func() { victim.Kill(killer); close(done) }) {
		t.Fatal("post: queue closed")
	}
	<-done
	srv.Settle(t)
	drain(t, gm)

	var ground []int32
	for _, g := range srv.GroundItems.Snapshots(nil) {
		ground = append(ground, g.TemplateID)
	}
	if len(ground) != 1 || ground[0] != keptPotionID {
		t.Fatalf("ground items = %v, want the potion alone", ground)
	}
	if got := srv.PlayerInventory(t, gmID).ItemCount(keptTunicID, -1, true); got != 1 {
		t.Fatalf("tunics kept = %d, want 1", got)
	}
}

// TestAdminReloadConfigTakesPartyRange: party loot after //reload config
// reads the reloaded party range, the one the kill rewards and the scripts
// read too, as the reference reads PARTY_RANGE on each use. A reload
// reading an unlimited range lets a member 3000 away share an adena pickup.
func TestAdminReloadConfigTakesPartyRange(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithDataReloads(fixedConfig(network.PlayerConfig{
			RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1,
			KillRewards: &manager.KillRewardSettings{PartyRange: -1, MultipleItemDrop: true},
		})))
	gm := srv.Client
	enterWorld(t, gm)
	member, memberID := addPlayer(t, srv, "member", "Member", userLevel)
	drain(t, gm)

	gm.Send(wireJoinParty("Member"))
	for range 50 {
		if frame := member.Read(); frame[0] == serverpackets.OpcodeAskJoinParty {
			break
		}
	}
	member.Send(wireAnswerJoinParty(1))
	drain(t, gm)
	drain(t, member)

	x, y, z := srv.PlayerPosition(t, gmID)
	onlineCharacter(t, srv, memberID).TeleportTo(x+3000, y, z, 0)
	drain(t, member)
	drain(t, gm)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload config")), configReloaded)

	before := srv.PlayerInventory(t, memberID).ItemCount(item.AdenaID, -1, true)
	srv.SeedGroundItem(t, 0, item.AdenaID, 100, x, y, z)
	snaps := srv.GroundItems.Snapshots(nil)
	if len(snaps) != 1 {
		t.Fatalf("ground items = %d, want 1", len(snaps))
	}
	exchange(t, gm, encodeAction(snaps[0].ObjectID))
	drain(t, gm)
	drain(t, member)

	if got := srv.PlayerInventory(t, gmID).ItemCount(item.AdenaID, -1, true); got != 50 {
		t.Fatalf("picker carries %d adena, want half of 100", got)
	}
	if got := srv.PlayerInventory(t, memberID).ItemCount(item.AdenaID, -1, true); got != before+50 {
		t.Fatalf("far member carries %d adena, want %d: the reloaded unlimited range shares the pickup", got, before+50)
	}
}

// wireJoinParty invites name into a finders-keepers party.
func wireJoinParty(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	w.WriteString(name)
	w.WriteInt32(0)
	return w.Bytes()
}

func wireAnswerJoinParty(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	w.WriteInt32(response)
	return w.Bytes()
}

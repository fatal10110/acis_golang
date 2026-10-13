package pets

import (
	"context"
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestReloadConfigReachesSummonOut: //reload config puts the reloaded
// MaxBuffsAmount and RespawnRestoreHP in force for a pet already out, as the
// reference reads players.properties on each use. Booted with 20 buff slots
// and a 70% revive, the pet holds 24 slots after the reload and revives at
// half its max HP.
func TestReloadConfigReachesSummonOut(t *testing.T) {
	t.Parallel()
	const gmLevel = 7
	adminData, err := admin.NewData([]admin.AccessLevel{{Level: 0}, {Level: gmLevel, IsGM: true, GiveDamage: true}},
		[]admin.Command{{Name: "admin_reload", AccessLevel: gmLevel}})
	if err != nil {
		t.Fatal(err)
	}
	reloaded := network.PlayerConfig{RespawnRestoreHP: 0.5, MaxBuffsAmount: 24, WeightLimitMultiplier: 1}
	srv := bootPets(t,
		gameservertest.WithAdmin(adminData),
		gameservertest.WithDataReloads(network.DataReloads{Config: func() (network.PlayerConfig, error) { return reloaded, nil }}),
	)
	ownerID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = ? WHERE obj_Id = ?", gmLevel, ownerID); err != nil {
		t.Fatalf("set access level: %v", err)
	}
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	startInWorld(t, srv.Client)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	wolf, _ := h.spawnWolf(t)
	if got := wolf.MaxBuffCount(); got != 20 {
		t.Fatalf("pet buff slots before the reload = %d, want 20", got)
	}

	frames := testsupport.SyncBarrierFrames(t, h.client, func() {
		h.client.Send(encodeBuildCmd("reload config"))
		barrier := wire.NewPacketWriter(clientpackets.OpcodeExtended)
		barrier.WriteUint16(clientpackets.OpcodeRequestManorList)
		h.client.Send(barrier.Bytes())
	}, serverpackets.OpcodeExtended)
	if !sawText(frames, "Configs files have been reloaded.") {
		t.Fatalf("//reload config frames = %x, want its reloaded text", frameOpcodes(frames))
	}

	if got := wolf.MaxBuffCount(); got != 24 {
		t.Fatalf("pet buff slots after the reload = %d, want 24", got)
	}
	killPet(t, h, wolf)
	runOn(t, wolf.Queue(), func() { wolf.Revive() })
	if want := math.Floor(wolf.MaxHPValue()) * 0.5; wolf.HP() != want {
		t.Fatalf("revived pet HP = %v, want %v (half of max)", wolf.HP(), want)
	}
	drainUntilQuiet(t, h.client)
}

// sawText reports whether frames hold a one-text SystemMessage reading text.
func sawText(frames [][]byte, text string) bool {
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != int32(serverpackets.SystemMessageS1) || r.ReadInt32() != 1 || r.ReadInt32() != int32(serverpackets.SystemMessageParamText) {
			continue
		}
		if r.ReadString() == text {
			return true
		}
	}
	return false
}

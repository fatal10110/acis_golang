package npcs

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestMultisellReloadedBlacksmithUseRecipes: //reload config puts the
// reloaded BlacksmithUseRecipes in force for the next exchange, as the
// reference reads it on each one. Booted with it off, three units of the
// entry keeping its ore need the one ore held; once a reload turns it on,
// the same exchange needs three and is refused.
func TestMultisellReloadedBlacksmithUseRecipes(t *testing.T) {
	t.Parallel()
	admin, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	reloads := network.DataReloads{Config: func() (network.PlayerConfig, error) {
		return network.PlayerConfig{RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1}, nil
	}}
	gm := func(srv *gameservertest.Server, _ map[int32]int32) {
		if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE obj_Id = ?", srv.SoleObjectID(t)); err != nil {
			t.Fatalf("set access level: %v", err)
		}
	}
	w := bootMultisell(t, msTalker(), [][2]int32{{item.AdenaID, 100}, {msOreID, 1}}, gm,
		gameservertest.WithKeepMaintainedIngredients(), gameservertest.WithAdmin(admin), gameservertest.WithDataReloads(reloads))
	w.mustOpen(t, "9001")
	assertMessages(t, w.choose(t, "9001", 5, 3),
		msg(serverpackets.SystemMessageS2S1Disappeared, item.AdenaID, 15),
		msg(serverpackets.SystemMessageEarnedS2S1S, msPotionID, 3),
		traded)

	cmd := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	cmd.WriteString("reload config")
	w.c.Send(cmd.Bytes())
	drainFrames(t, w.c)

	w.mustOpen(t, "9001")
	assertMessages(t, w.choose(t, "9001", 5, 3), msg(serverpackets.SystemMessageNotEnoughItems))
	if ore, adena := w.held(t, msOreID), w.held(t, item.AdenaID); ore != 1 || adena != 85 {
		t.Fatalf("ore %d adena %d, want 1 and 85", ore, adena)
	}
}

package items

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/enchant"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// reloadingGM lets the character objID run //reload, whose config reload
// reads settings: the shipped admin commands, and a game master's access
// level set before it enters the world.
func reloadingGM(t *testing.T, settings network.PlayerConfig) []gameservertest.Option {
	t.Helper()
	admin, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	return []gameservertest.Option{
		gameservertest.WithAdmin(admin),
		gameservertest.WithDataReloads(network.DataReloads{Config: func() (network.PlayerConfig, error) { return settings, nil }}),
	}
}

func makeGM(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE obj_Id = ?", objID); err != nil {
		t.Fatalf("set access level: %v", err)
	}
}

// reloadConfig runs //reload config for c and reads its answer.
func reloadConfig(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	w.WriteString("reload config")
	c.Send(w.Bytes())
	drainUntilQuiet(t, c)
}

// TestReloadConfigEnchantRates: //reload config puts the reloaded scroll
// rates in force for the next enchant, as the reference reads them on each
// one. A weapon at +3 rolling 0.5 would succeed at the shipped 70%; the
// reloaded 30% breaks it.
func TestReloadConfigEnchantRates(t *testing.T) {
	t.Parallel()
	rates := enchant.DefaultConfig()
	rates.ChanceWeapon = 0.3
	srv, objID, weapon, scroll := bootEnchanter(t, func() float64 { return 0.5 }, 3, false, makeGM,
		reloadingGM(t, network.PlayerConfig{Enchant: &rates, RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1})...)
	c := srv.Client
	reloadConfig(t, c)

	openEnchantSelection(t, c, scroll, 955)
	c.Send(encodeRequestEnchantItem(weapon))
	for {
		frame := c.Read()
		if frame[0] == serverpackets.OpcodeEnchantResult {
			assertEnchantResult(t, frame, serverpackets.EnchantResultBrokenWithCrystals)
			break
		}
	}
	srv.FlushItems(t)
	assertItemGone(t, srv, objID, weapon)
}

// TestReloadConfigCraftingEnabled: //reload config puts the reloaded
// CraftingEnabled in force for the next recipe registered. Booted with
// crafting off, the recipe is refused; once a reload turns it on, the same
// recipe enters the book.
func TestReloadConfigCraftingEnabled(t *testing.T) {
	t.Parallel()
	srv, objID := bootCraft(t, append(reloadingGM(t, network.PlayerConfig{RespawnRestoreHP: 0.7, MaxBuffsAmount: 20, WeightLimitMultiplier: 1}),
		gameservertest.WithCraftingDisabled())...)
	c := srv.Client
	knowSkill(t, srv, objID, modelskill.CreateCommonSkillID)
	makeGM(t, srv, objID)
	scroll := srv.GiveItem(t, objID, commonRecipeItemID, 1)
	startInWorld(t, c)

	c.Send(encodeUseItem(scroll, false))
	if id, text := textSysMsg(t, c.Read()); id != serverpackets.SystemMessageS1 || text != "Crafting is disabled, you cannot register this recipe." {
		t.Fatalf("register before the reload = %d %q", id, text)
	}
	barrier(t, c)

	reloadConfig(t, c)
	c.Send(encodeUseItem(scroll, false))
	drainUntilQuiet(t, c)
	if got := savedRecipes(t, srv, objID); len(got) != 1 || got[0] != commonRecipeID {
		t.Fatalf("saved recipes after the reload = %v, want [%d]", got, commonRecipeID)
	}
}

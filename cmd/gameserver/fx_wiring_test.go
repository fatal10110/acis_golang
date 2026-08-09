package main

import (
	"testing"

	"github.com/rs/zerolog"
	"go.uber.org/fx"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestProvideGameClientLinkUsesGameDataSkillTrees(t *testing.T) {
	err := fx.ValidateApp(
		fx.Provide(
			func() gameServerConfig { return gameServerConfig{} },
			func() *gameData { return &gameData{} },
			func() *manager.Roster { return nil },
			func() *gamesql.ItemStore { return nil },
			func() *gamesql.ShortcutStore { return nil },
			func() *gamesql.PetStore { return nil },
			func() *datacache.HTML { return nil },
			func() *datacache.Crests { return nil },
			func() *network.SessionValidator { return nil },
			func() *loginLinkState { return nil },
			func() *skillstate.Persistence { return nil },
			func() modelskill.BookPolicy { return modelskill.BookPolicy{} },
			func() *world.State { return nil },
			func() *idfactory.Allocator { return nil },
			func() *task.GroundItems { return nil },
			func() *task.AttackStance { return nil },
			func() *task.PvPFlags { return nil },
			func() task.PvPFlagOptions { return task.DefaultPvPFlagOptions() },
			func() *task.PositionUpdates { return nil },
			func() *task.InventoryUpdates { return nil },
			func() *task.ItemInstances { return nil },
			func() *task.PlayerClock { return nil },
			func() *task.Water { return nil },
			func() *task.ShadowItems { return nil },
			func() *task.Autosave { return nil },
			func() *network.TaskEffects { return nil },
			func() respawnRestoreHP { return 0.7 },
			func() skillEnchantSPBookNeeded { return true },
			func() autoLearnSkills { return false },
			func() weightLimitMultiplier { return 1 },
			func() karmaPlayerCanTeleport { return true },
			func() pet.Config { return pet.DefaultConfig() },
			func() zerolog.Logger { return zerolog.Nop() },
			provideGameClientLink,
		),
		fx.Invoke(func(*network.GameClientLink) {}),
	)
	if err != nil {
		t.Fatalf("provideGameClientLink graph = %v", err)
	}
}

package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

type loginLinkState struct {
	link atomic.Pointer[network.LoginLink]
}

func provideLoginLinkState() *loginLinkState {
	return &loginLinkState{}
}

func (s *loginLinkState) set(link *network.LoginLink) {
	s.link.Store(link)
}

func (s *loginLinkState) clear(link *network.LoginLink) {
	s.link.CompareAndSwap(link, nil)
}

func (s *loginLinkState) get() *network.LoginLink {
	return s.link.Load()
}

func provideGameClientLink(
	data *gameData,
	roster *manager.Roster,
	items *gamesql.ItemStore,
	shortcuts *gamesql.ShortcutStore,
	html *datacache.HTML,
	crests *datacache.Crests,
	validator *network.SessionValidator,
	links *loginLinkState,
	skills *skillstate.Persistence,
	spellbooks skill.BookPolicy,
	state *world.State,
	ids *idfactory.Allocator,
	ground *task.GroundItems,
	attackStance *task.AttackStance,
	positions *task.PositionUpdates,
	playerClock *task.PlayerClock,
	inventoryUpdates *task.InventoryUpdates,
	itemInstances *task.ItemInstances,
	respawnHP respawnRestoreHP,
	spBookNeeded skillEnchantSPBookNeeded,
	autoLearn autoLearnSkills,
	weightLimit weightLimitMultiplier,
	karmaTeleport karmaPlayerCanTeleport,
	petCfg pet.Config,
	log zerolog.Logger,
) *network.GameClientLink {
	playerConfig := network.PlayerConfig{
		RespawnRestoreHP:         float64(respawnHP),
		SkillEnchantSPBookNeeded: bool(spBookNeeded),
		AutoLearnSkills:          bool(autoLearn),
		WeightLimitMultiplier:    float64(weightLimit),
		KarmaPlayerCanTeleport:   bool(karmaTeleport),
	}
	return network.NewGameClientLink(network.GameClientLinkConfig{
		Validator:     validator,
		LoginLink:     links.get,
		Roster:        roster,
		Items:         items,
		Shortcuts:     shortcuts,
		Templates:     data.Players,
		ItemTemplates: data.Items,
		HTML:          html,
		Crests:        crests,
		Skills:        skills,
		Spellbooks:    spellbooks,
		SkillTrees:    data.Trees,
		CursedWeapons: data.CursedWeapons,
		World:         state,
		NPCs:          data.NPCs,
		Geo:           move.NewGeo(data.Geo, data.Finder),
		Zones:         data.Zones,
		IDs:           ids,
		GroundItems:   ground,
		AttackStance:  attackStance,
		Positions:     positions,
		PlayerClock:   playerClock,

		InventoryUpdates: inventoryUpdates,
		ItemInstances:    itemInstances,
		Restarts:         data.Restarts,
		Levels:           data.Levels,
		PlayerConfig:     playerConfig,
		PetConfig:        petCfg,
		Log:              log,
	})
}

func provideSkillPersistence(pool *sql.DB, data *gameData) *skillstate.Persistence {
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(pool), data.Skills, gamesql.NewCharacterSkillStore(pool))
}

func startGameServer(lc fx.Lifecycle, cfg gameServerConfig, _ *gameData, _ *manager.Roster, validator *network.SessionValidator, links *loginLinkState, clients *network.GameClientLink, log zerolog.Logger) {
	var cancel context.CancelFunc
	var wg sync.WaitGroup
	wroteGeneratedHexID := false

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			runCtx, stop := context.WithCancel(context.Background())
			cancel = stop

			wg.Add(1)
			go func() {
				defer wg.Done()
				network.Maintain(runCtx, cfg.LoginAddr, cfg.Auth, network.LoginLinkHandlers{
					PlayerAuthResponse: validator.Resolve,
				}, network.DefaultReconnectDelay, func(link *network.LoginLink) {
					links.set(link)
					if cfg.GeneratedHexID && !wroteGeneratedHexID {
						if err := writeHexIDFile(cfg.HexIDPath, int(link.ServerID), cfg.Auth.HexID); err != nil {
							log.Error().Err(err).Str("path", cfg.HexIDPath).Msg("write generated hexid")
						} else {
							wroteGeneratedHexID = true
							log.Info().Str("path", cfg.HexIDPath).Int("server_id", int(link.ServerID)).Msg("generated hexid saved")
						}
					}
					go func() {
						<-link.Done()
						links.clear(link)
					}()
					log.Info().Int("server_id", int(link.ServerID)).Str("name", link.ServerName).Msg("linked to loginserver")
				}, log)
			}()

			ln, err := net.Listen("tcp", cfg.ListenAddr)
			if err != nil {
				stop()
				return fmt.Errorf("listen for game clients on %s: %w", cfg.ListenAddr, err)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := network.Serve(runCtx, ln, clients.Handle, log); err != nil {
					log.Error().Err(err).Str("addr", cfg.ListenAddr).Msg("game client listener stopped")
				}
			}()
			log.Info().Str("addr", cfg.ListenAddr).Msg("listening for game clients")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if cancel != nil {
				cancel()
			}
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			select {
			case <-done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
}

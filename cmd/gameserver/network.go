package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
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
	cfg gameServerConfig,
	data *gameData,
	roster *manager.Roster,
	items *gamesql.ItemStore,
	shortcuts *gamesql.ShortcutStore,
	hennas *gamesql.HennaStore,
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
	ai *task.AI,
	pvpFlags *task.PvPFlags,
	pvpOptions task.PvPFlagOptions,
	positions *task.PositionUpdates,
	playerClock *task.PlayerClock,
	gameClock *task.GameClock,
	sevenSigns *sevensigns.State,
	inventoryUpdates *task.InventoryUpdates,
	itemInstances *task.ItemInstances,
	water *task.Water,
	shadowItems *task.ShadowItems,
	autosave *task.Autosave,
	effects *network.TaskEffects,
	respawnHP respawnRestoreHP,
	deathPenalty deathPenaltyChance,
	shieldBlockRate perfectShieldBlockRate,
	spawnProtection playerSpawnProtection,
	spBookNeeded skillEnchantSPBookNeeded,
	autoLearn autoLearnSkills,
	weightLimit weightLimitMultiplier,
	karmaTeleport karmaPlayerCanTeleport,
	delevel allowDelevel,
	karmaExpLost rateKarmaExpLost,
	charSelect characterSelectDelay,
	bypassDelay serverBypassDelay,
	petCfg pet.Config,
	petStore *gamesql.PetStore,
	log zerolog.Logger,
) *network.GameClientLink {
	playerConfig := network.PlayerConfig{
		RespawnRestoreHP:         float64(respawnHP),
		DeathPenaltyChance:       int(deathPenalty),
		PerfectShieldBlockRate:   int(shieldBlockRate),
		SpawnProtection:          time.Duration(spawnProtection),
		SkillEnchantSPBookNeeded: bool(spBookNeeded),
		AutoLearnSkills:          bool(autoLearn),
		WeightLimitMultiplier:    float64(weightLimit),
		KarmaPlayerCanTeleport:   bool(karmaTeleport),
		AwardPKKillPVPPoint:      pvpOptions.AwardPKKillPVPPoint,
		AllowWater:               cfg.AllowWater,
		AllowDelevel:             bool(delevel),
		RateKarmaExpLost:         float64(karmaExpLost),
		CharacterSelectDelay:     time.Duration(charSelect),
		ServerBypassDelay:        time.Duration(bypassDelay),
	}
	link := network.NewGameClientLink(network.GameClientLinkConfig{
		Validator:     validator,
		NoCipher:      !cfg.UseBlowfishCipher,
		LoginLink:     links.get,
		Roster:        roster,
		Items:         items,
		Shortcuts:     shortcuts,
		Hennas:        hennas,
		HennaTable:    data.Hennas,
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
		SummonItems:   data.SummonItems,
		PetStore:      petStore,
		Geo:           move.NewGeo(data.Geo, data.Finder),
		Zones:         data.Zones,
		IDs:           ids,
		GroundItems:   ground,
		AttackStance:  attackStance,
		AI:            ai,
		PvPFlags:      pvpFlags,
		Positions:     positions,
		PlayerClock:   playerClock,
		GameClock:     gameClock,
		SevenSigns:    sevenSigns,
		Water:         water,
		ShadowItems:   shadowItems,
		Autosave:      autosave,

		InventoryUpdates: inventoryUpdates,
		ItemInstances:    itemInstances,
		Restarts:         data.Restarts,
		Levels:           data.Levels,
		Admin:            data.Admin,
		PlayerConfig:     playerConfig,
		PetConfig:        petCfg,
		Log:              log,
	})
	if effects != nil {
		effects.SetShadowItemExpiry(link.ExpireShadowItem)
	}
	return link
}

func provideSkillPersistence(pool *sql.DB, data *gameData) *skillstate.Persistence {
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(pool), data.Skills, gamesql.NewCharacterSkillStore(pool))
}

// onlineAccounts collects the account names of every player currently in
// the world, forming the roster reported to the login server at
// registration.
func onlineAccounts(state *world.State) []string {
	var out []string
	for _, obj := range state.Players() {
		p, ok := obj.(interface{ AccountName() string })
		if !ok {
			continue
		}
		if name := p.AccountName(); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// playerAuthResponseHandler reports a successfully authed account to the
// login server before resolving the waiting game client, so the online
// roster grows before the client proceeds into the world.
func playerAuthResponseHandler(links *loginLinkState, validator *network.SessionValidator, log zerolog.Logger) func(string, bool) {
	return func(account string, ok bool) {
		if link := links.get(); ok && link != nil {
			if err := link.SendPlayerInGame([]string{account}); err != nil {
				log.Debug().Err(err).Str("account", account).Msg("notify player in game")
			}
		}
		validator.Resolve(account, ok)
	}
}

func startGameServer(lc fx.Lifecycle, cfg gameServerConfig, _ *gameData, _ *manager.Roster, validator *network.SessionValidator, links *loginLinkState, clients *network.GameClientLink, state *world.State, log zerolog.Logger) {
	var cancel context.CancelFunc
	var wg sync.WaitGroup
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			runCtx, stop := context.WithCancel(context.Background())
			cancel = stop

			wg.Add(1)
			go func() {
				defer wg.Done()
				network.Maintain(runCtx, cfg.LoginAddr, cfg.Auth, network.LoginLinkHandlers{
					PlayerAuthResponse: playerAuthResponseHandler(links, validator, log),
				}, network.DefaultReconnectDelay, func(link *network.LoginLink) {
					links.set(link)
					if accounts := onlineAccounts(state); len(accounts) > 0 {
						if err := link.SendPlayerInGame(accounts); err != nil {
							log.Error().Err(err).Int("accounts", len(accounts)).Msg("send online roster to loginserver")
						} else {
							log.Info().Int("accounts", len(accounts)).Msg("online roster sent to loginserver")
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

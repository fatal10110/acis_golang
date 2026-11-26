package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/debughttp"
	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/commons/netutil"
	"github.com/fatal10110/acis_golang/internal/gameserver/announcement"
	"github.com/fatal10110/acis_golang/internal/gameserver/boat"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/cursedweapon"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/derby"
	"github.com/fatal10110/acis_golang/internal/gameserver/fence"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
	"github.com/fatal10110/acis_golang/internal/gameserver/hero"
	"github.com/fatal10110/acis_golang/internal/gameserver/itemapi"
	"github.com/fatal10110/acis_golang/internal/gameserver/lottery"
	"github.com/fatal10110/acis_golang/internal/gameserver/merchant"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/raidpoint"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/shutdown"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameserver/signspriest"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/relation"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
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

// gameClientLinkDeps is what the game client link is built from, in the
// order the graph resolves it.
type gameClientLinkDeps struct {
	fx.In

	Cfg              gameServerConfig
	Data             *gameData
	Roster           *manager.Roster
	Items            *gamesql.ItemStore
	Shortcuts        *gamesql.ShortcutStore
	Hennas           *gamesql.HennaStore
	RecipeBooks      *gamesql.RecipeBookStore
	Macros           *gamesql.MacroStore
	Bookmarks        *gamesql.BookmarkStore
	Recommendations  *gamesql.RecommendationStore
	Quests           *gamesql.QuestStore
	Scripts          *script.Registry
	Journals         *script.Quests
	Memos            *gamesql.MemoStore
	Subclasses       *gamesql.SubclassStore
	Shops            *merchant.Service
	HTML             *datacache.HTML
	Crests           *datacache.Crests
	Validator        *network.SessionValidator
	Links            *loginLinkState
	Skills           *skillstate.Persistence
	Spellbooks       skill.BookPolicy
	State            *world.State
	WorldObjects     *manager.WorldObjects
	IDs              *idfactory.Allocator
	Ground           *task.GroundItems
	AttackStance     *task.AttackStance
	AI               *task.AI
	PvPFlags         *task.PvPFlags
	Decay            *task.Decay
	PvPOptions       task.PvPFlagOptions
	Positions        *task.PositionUpdates
	PlayerClock      *task.PlayerClock
	GameClock        *task.GameClock
	SevenSigns       *sevensigns.State
	Fest             *festival.Manager
	SignsPriest      signspriest.Config
	OlympiadState    *olympiad.Olympiad
	Heroes           *hero.Manager
	RaidPoints       *raidpoint.Points
	CursedWeapons    *cursedweapon.Manager
	InventoryUpdates *task.InventoryUpdates
	ItemInstances    *task.ItemInstances
	Water            *task.Water
	ShadowItems      *task.ShadowItems
	Autosave         *task.Autosave
	Effects          *network.TaskEffects
	EffectEnv        effect.Env
	Gameplay         gameplayConfig
	Rewards          manager.KillRewardConfig
	Events           *script.EventBook
	QuestRates       *script.RateBook
	PetCfg           pet.Config
	PetStore         *gamesql.PetStore
	Worker           *persist.Worker
	ItemWrites       *persist.Order
	Pool             *sim.Pool
	Relations        *relation.Manager
	Petitions        *petition.Manager
	Characters       *gamesql.CharacterStore
	Clans            *clan.Service
	Castles          *castle.Manager
	CastleManor      *castlemanor.Manager
	Sieges           *siege.Engine
	Board            communityBoard
	Announcements    *announcement.Registry
	SchemeBuffer     *schemebuffer.Manager
	Couples          *wedding.Manager
	LotteryState     *lottery.Lottery
	FishChamp        *fishchamp.Championship
	DerbyTrack       *derby.Track
	Fleet            *boat.Fleet
	HallFunctions    *clanhall.Functions
	Halls            *clanhall.Halls
	Log              zerolog.Logger
	GMAudit          gmAuditLogger
	Reloads          network.DataReloads
	BossZones        *gamesql.BossZoneStore
	LoginStatus      *network.LoginStatus
	Countdown        *shutdown.Countdown
	ChatLog          chatLogger
	ItemLog          itemLogger
}

func provideGameClientLink(d gameClientLinkDeps) (*network.GameClientLink, error) {
	playerConfig := newPlayerConfig(d.Cfg, d.Gameplay, d.PvPOptions)
	link, err := network.NewGameClientLink(network.GameClientLinkConfig{
		Flow:          d.flowConfig(),
		Belongings:    d.belongingsConfig(),
		Combat:        d.combatConfig(),
		Summoning:     d.summoningConfig(),
		Enchanting:    network.EnchantingConfig{ArmorSets: d.Data.ArmorSets},
		Effects:       d.EffectEnv,
		LoginLink:     d.Links.get,
		Roster:        d.Roster,
		Subclasses:    d.Subclasses,
		Recipes:       d.Data.Recipes,
		Merchant:      d.Shops,
		Multisells:    d.Data.Multisells,
		Templates:     d.Data.Players,
		ItemTemplates: d.Data.Items,
		HTML:          d.HTML,
		Crests:        d.Crests,
		Skills:        d.Skills,
		Spellbooks:    d.Spellbooks,
		SkillTrees:    d.Data.Trees,
		CursedWeapons: d.Data.CursedWeapons,
		CursedRuntime: d.CursedWeapons,
		World:         d.State,
		NPCs:          d.Data.NPCs,
		Doors:         d.WorldObjects,
		Geo:           move.NewGeo(d.Data.Geo, d.Data.Finder),
		GeoBugs:       d.Data.GeoBugs,
		Zones:         d.Data.Zones,
		IDs:           d.IDs,
		Fences:        fence.NewManager(d.Data.Geo, d.State, d.IDs),
		Positions:     d.Positions,
		GameClock:     d.GameClock,
		SevenSigns:    d.SevenSigns,
		Festival:      d.Fest,
		SignsPriest:   &d.SignsPriest,
		Olympiad:      d.OlympiadState,
		Heroes:        d.Heroes,
		RaidPoints:    d.RaidPoints,
		Water:         d.Water,
		ShadowItems:   d.ShadowItems,

		Persist:          d.Worker,
		ItemWrites:       d.ItemWrites,
		Queues:           d.Pool,
		Restarts:         d.Data.Restarts,
		ClanHallData:     d.Data.ClanHalls,
		CastleData:       d.Data.Castles,
		Observers:        d.Data.Observers,
		Levels:           d.Data.Levels,
		Admin:            d.Data.Admin,
		GMAudit:          d.GMAudit.enabled(d.Gameplay.Admin.GMAudit),
		ItemLog:          d.ItemLog.enabled(bool(d.Gameplay.LogItems)),
		Chat:             d.chatConfig(),
		PlayerConfig:     playerConfig,
		DisableRaidCurse: bool(d.Gameplay.DisableRaidCurse),
		ShowNPCCrest:     bool(d.Gameplay.ShowNPCCrest),
		ShowSummonCrest:  bool(d.Gameplay.ShowSummonCrest),
		Wyvern:           &d.Gameplay.Wyvern,
		Macros:           d.Macros,
		Bookmarks:        d.Bookmarks,
		Recommendations:  d.Recommendations,
		Quests:           d.Quests,
		Scripts:          d.Scripts,
		Journals:         d.Journals,
		Memos:            d.Memos,
		Teleports:        d.Data.Teleports,
		InstantTeleports: d.Data.InstantTeleports,
		FreeTeleport:     bool(d.Gameplay.FreeTeleport),
		ClassMaster:      d.Gameplay.ClassMaster,
		Log:              d.Log,

		Augmentations:       d.Data.Augmentations,
		AugmentationChances: d.Gameplay.AugmentationChances,
		Manor:               manorConfig(d.Gameplay.Manor, d.Data),
		Fish:                d.Data.Fish,

		Relations:      d.Relations,
		Characters:     d.Characters,
		Clans:          d.Clans,
		Castles:        d.Castles,
		CastleManor:    d.CastleManor,
		Sieges:         d.Sieges,
		AccessLevels:   d.Characters,
		CharacterEdits: d.Characters,
		Punishments:    d.Characters,
		Petitions:      d.Petitions,

		Board:               d.Board.Config,
		Mailbox:             d.Board.Mailbox,
		Forums:              d.Board.Forums,
		Favorites:           d.Board.Favorites,
		ShowServerNews:      d.Board.ShowServerNews,
		Announcements:       d.Announcements,
		SchemeBuffer:        d.SchemeBuffer,
		Wedding:             d.Couples,
		Lottery:             d.LotteryState,
		FishingChampionship: d.FishChamp,
		Derby:               d.DerbyTrack,
		Boats:               d.Fleet,

		ClanHallFunctions: d.HallFunctions,
		ClanHalls:         d.Halls,

		Reloads:     d.Reloads,
		KillRewards: d.Rewards.Settings,
		Events:      d.Events,
		QuestRates:  d.QuestRates,
		BossZones:   d.BossZones,

		Shutdown:    d.Countdown,
		LoginStatus: d.LoginStatus,
	})
	if err != nil {
		return nil, err
	}
	if d.Effects != nil {
		d.Effects.SetShadowItemExpiry(link.ExpireShadowItem)
	}
	d.WorldObjects.SetRegenSettings(link.PlayerRules())
	return link, nil
}

// flowConfig is the character flow part of the link config.
func (d *gameClientLinkDeps) flowConfig() network.CharacterFlowConfig {
	return network.CharacterFlowConfig{
		Validator:   d.Validator,
		NoCipher:    !d.Cfg.UseBlowfishCipher,
		PlayerClock: d.PlayerClock,
		Autosave:    d.Autosave,
	}
}

// belongingsConfig is the item, shortcut, henna and recipe part of the link
// config.
func (d *gameClientLinkDeps) belongingsConfig() network.BelongingsConfig {
	return network.BelongingsConfig{
		Items:            d.Items,
		Shortcuts:        d.Shortcuts,
		Hennas:           d.Hennas,
		HennaTable:       d.Data.Hennas,
		RecipeBooks:      d.RecipeBooks,
		GroundItems:      d.Ground,
		InventoryUpdates: d.InventoryUpdates,
		ItemInstances:    d.ItemInstances,
	}
}

// combatConfig is the combat part of the link config.
func (d *gameClientLinkDeps) combatConfig() network.CombatConfig {
	return network.CombatConfig{
		AttackStance: d.AttackStance,
		AI:           d.AI,
		PvPFlags:     d.PvPFlags,
		HealSps:      d.Data.HealSps,
	}
}

// summoningConfig is the pet and servitor part of the link config.
func (d *gameClientLinkDeps) summoningConfig() network.SummoningConfig {
	return network.SummoningConfig{
		Items:  d.Data.SummonItems,
		Store:  d.PetStore,
		Config: d.PetCfg,
		Decay:  d.Decay,
	}
}

// chatConfig is the chat part of the link config.
func (d *gameClientLinkDeps) chatConfig() network.ChatConfig {
	return network.ChatConfig{
		Log:              d.ChatLog.enabled(d.Gameplay.Chat.LogChat),
		WalkerProtection: d.Gameplay.Chat.WalkerProtection,
		GlobalDelay:      d.Gameplay.Chat.GlobalDelay,
		TradeDelay:       d.Gameplay.Chat.TradeDelay,
		HeroVoiceDelay:   d.Gameplay.Chat.HeroVoiceDelay,
	}
}

func provideSkillPersistence(pool *sql.DB, data *gameData, gameplay gameplayConfig, worker *persist.Worker, log zerolog.Logger) (*skillstate.Persistence, error) {
	skills := skillstate.NewPersistenceWithStoreSkillCooltime(gamesql.NewSkillSaveStore(pool), data.Skills, bool(gameplay.StoreSkillCooltime), gamesql.NewCharacterSkillStore(pool))
	// A learn, enchant or level refresh runs on the player's queue; its
	// character_skills write must not block that queue.
	skills.SetPersistWorker(worker, log)
	if err := skills.SetAugmentations(data.Augmentations); err != nil {
		return nil, err
	}
	skills.SetArmorSets(data.ArmorSets)
	return skills, nil
}

// onlineAccounts collects the account names of every player currently in
// the world, forming the roster reported to the login server at
// registration.
func onlineAccounts(state *world.State) []string {
	var out []string
	for _, obj := range state.Players() {
		if name := obj.AccountName(); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// onLoginLink returns what runs each time the login link comes up: it
// publishes the link, writes the registration the login server confirmed
// to the hexid file the next boot reads, and reports every account in the
// world as the online roster, in as many messages as it needs. A failed
// hexid write is logged and keeps the link.
func onLoginLink(links *loginLinkState, status *network.LoginStatus, state *world.State, hexIDPath string, hexID []byte, log zerolog.Logger) func(*network.LoginLink) {
	return func(ll *network.LoginLink) {
		links.set(ll)
		status.Registered(ll.ServerName)
		if err := link.WriteHexIDFile(hexIDPath, int(ll.ServerID), hexID, time.Now()); err != nil {
			log.Error().Err(err).Str("path", hexIDPath).Msg("save hexid")
		}
		if accounts := onlineAccounts(state); len(accounts) > 0 {
			if err := ll.SendPlayerInGame(accounts); err != nil {
				log.Error().Err(err).Int("accounts", len(accounts)).Msg("send online roster to loginserver")
			} else {
				log.Info().Int("accounts", len(accounts)).Msg("online roster sent to loginserver")
			}
		}
		go func() {
			<-ll.Done()
			links.clear(ll)
		}()
		log.Info().Int("server_id", int(ll.ServerID)).Str("name", ll.ServerName).Msg("linked to loginserver")
	}
}

// clientBans is the game-client port's IP ban list and accept filter,
// shared by the listener and the ban API.
type clientBans struct {
	bans   *netutil.BanList
	filter *netutil.AcceptFilter
}

func provideClientBans(cfg gameServerConfig, log zerolog.Logger) *clientBans {
	return &clientBans{bans: netutil.NewBanList(time.Now), filter: netutil.NewAcceptFilter(cfg.AcceptFilter, log)}
}

func startDebugHTTP(lc fx.Lifecycle, paths gameServerPaths, state *world.State, bans *clientBans, link *network.GameClientLink, log zerolog.Logger) {
	debughttp.SetPlayersOnlineFunc(func() int { return len(state.Players()) })
	if paths.DebugAddr == "" {
		return
	}
	api := debughttp.BanHandler("game", []debughttp.BanStore{
		{Name: "game-ban", Bans: bans.bans},
		{Name: "game-accept-filter", Bans: bans.filter},
	}, log)
	var srv *http.Server
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			var err error
			srv, err = debughttp.Listen(paths.DebugAddr,
				debughttp.Route{Path: debughttp.BansPath, Handler: api},
				debughttp.Route{Path: itemapi.Path, Handler: itemapi.Handler(link)})
			if err != nil {
				return fmt.Errorf("listen for debug http on %s: %w", paths.DebugAddr, err)
			}
			log.Info().Str("addr", paths.DebugAddr).Msg("debug http listening")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, debugHTTPStopTimeout)
			defer cancel()
			return debughttp.Shutdown(ctx, srv)
		},
	})
}

func startGameServer(lc fx.Lifecycle, cfg gameServerConfig, paths gameServerPaths, _ *gameData, _ *manager.Roster, validator *network.SessionValidator, links *loginLinkState, status *network.LoginStatus, clients *network.GameClientLink, state *world.State, bans *clientBans, log zerolog.Logger) {
	var cancel context.CancelFunc
	var wg sync.WaitGroup
	lc.Append(fx.Hook{
		OnStart: func(startCtx context.Context) error {
			runCtx, stop := context.WithCancel(context.Background())
			cancel = stop

			wg.Add(1)
			go func() {
				defer wg.Done()
				auth := func() network.LoginServerAuth { return status.Auth(cfg.Auth) }
				network.Maintain(runCtx, cfg.LoginAddr, auth, network.LoginLinkHandlers{
					PlayerAuthResponse: network.PlayerAuthAnswers(validator, links.get, log),
					KickPlayer:         clients.KickPlayer,
				}, network.DefaultReconnectDelay, onLoginLink(links, status, state, paths.HexIDPath, cfg.Auth.HexID, log), log)
			}()

			addr := gameBindAddress(startCtx, cfg.ListenAddr, net.DefaultResolver.LookupIPAddr, log)
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				stop()
				return fmt.Errorf("listen for game clients on %s: %w", addr, err)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Banned addresses, then connection bursts from one
				// address, are refused at accept; a banned address's
				// attempts do not count toward a burst.
				filtered := netutil.FilterAccepts(netutil.RefuseBanned(ln, bans.bans), bans.filter, time.Now)
				if err := network.Serve(runCtx, filtered, clients.Handle, log); err != nil {
					log.Error().Err(err).Str("addr", addr).Msg("game client listener stopped")
				}
			}()
			log.Info().Str("addr", addr).Msg("listening for game clients")
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

// newPlayerConfig gathers the settings the game client link reads at each
// use from the server and players properties, at boot and on
// //reload config.
func newPlayerConfig(cfg gameServerConfig, gameplay gameplayConfig, pvpOptions task.PvPFlagOptions) network.PlayerConfig {
	enchantCfg := gameplay.Enchant
	freightCfg := gameplay.Freight
	return network.PlayerConfig{
		Enchant:                    &enchantCfg,
		RespawnRestoreHP:           float64(gameplay.RespawnRestoreHP),
		DeathPenaltyChance:         int(gameplay.DeathPenaltyChance),
		MaxBuffsAmount:             int(gameplay.MaxBuffsAmount),
		ShowNPCLevel:               bool(gameplay.ShowNPCLevel),
		MagicFailures:              bool(gameplay.MagicFailures),
		PerfectShieldBlockRate:     int(gameplay.PerfectShieldBlockRate),
		SpawnProtection:            time.Duration(gameplay.SpawnProtection),
		SkillEnchantSPBookNeeded:   bool(gameplay.SkillEnchantSPBookNeeded),
		AutoLearnSkills:            bool(gameplay.AutoLearnSkills),
		WeightLimitMultiplier:      float64(gameplay.WeightLimitMultiplier),
		InventorySlots:             gameplay.InventorySlots,
		Regen:                      gameplay.RegenMultipliers,
		StorageSlots:               gameplay.StorageSlots,
		Freight:                    &freightCfg,
		KarmaPlayerCanTeleport:     bool(gameplay.KarmaPlayerCanTeleport),
		KarmaPlayerCanShop:         gameplay.KarmaServiceGates.CanShop,
		KarmaPlayerCanUseGK:        gameplay.KarmaServiceGates.CanUseGK,
		KarmaPlayerCanUseWareHouse: gameplay.KarmaServiceGates.CanUseWareHouse,
		KarmaPlayerCanTrade:        bool(gameplay.KarmaPlayerCanTrade),
		AwardPKKillPVPPoint:        pvpOptions.AwardPKKillPVPPoint,
		AllowWater:                 cfg.AllowWater,
		EnableFallingDamage:        cfg.EnableFallingDamage,
		AllowDelevel:               bool(gameplay.AllowDelevel),
		RateKarmaExpLost:           float64(gameplay.RateKarmaExpLost),
		DeathDrop:                  gameplay.DeathDrop,
		CharacterSelectDelay:       time.Duration(gameplay.CharacterSelectDelay),
		ServerBypassDelay:          time.Duration(gameplay.ServerBypassDelay),
		CraftingDisabled:           !bool(gameplay.CraftingEnabled),
		ManufactureDelay:           time.Duration(gameplay.ManufactureDelay),
		MultisellDelay:             time.Duration(gameplay.MultisellDelay),
		ManorDelay:                 time.Duration(gameplay.ManorDelay),
		RollDiceDelay:              time.Duration(gameplay.RollDiceDelay),
		SocialDelay:                time.Duration(gameplay.SocialDelay),
		DropItemDelay:              time.Duration(gameplay.DropItemDelay),
		WarehouseDisabled:          !bool(gameplay.WarehouseAllowed),
		FakeDeathUpProtection:      time.Duration(gameplay.FakeDeathUpProtection),
		SubclassDelay:              time.Duration(gameplay.SubclassDelay),
		SubclassWithoutQuests:      bool(gameplay.SubclassWithoutQuests),
		KeepMaintainedIngredients:  !bool(gameplay.BlacksmithUseRecipes),
		DiscardItemDisabled:        !bool(gameplay.AllowDiscardItem),
		GMStartupUnlisted:          !gameplay.Admin.GMStartupAutoList,
		GMStartupInvulnerable:      gameplay.Admin.GMStartupInvulnerable,
		GMStartupInvisible:         gameplay.Admin.GMStartupInvisible,
		GMStartupBlockAll:          gameplay.Admin.GMStartupBlockAll,
		GMHeroAura:                 gameplay.Admin.GMHeroAura,
		DefaultAccessLevel:         gameplay.Admin.DefaultAccessLevel,
	}
}

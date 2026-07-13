// Command gameserver boots the game server process.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/fx"

	"github.com/fatal10110/acis_golang/internal/commons/db"
	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/commons/logging"
	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/probe"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
)

const generatedHexIDSize = 16

type gameServerPaths struct {
	ConfigPath        string
	LoggingPath       string
	PlayersConfigPath string
	HexIDPath         string
	GeoConfigPath     string
	DataRoot          string
	LogRoot           string
}

type gameServerConfig struct {
	ListenAddr     string
	LoginAddr      string
	Auth           network.LoginServerAuth
	GeneratedHexID bool
	HexIDPath      string
	Database       db.Config
}

type gameData struct {
	Players *player.TemplateTable
	Levels  *player.LevelTable
	Items   *item.Table
	Skills  *skill.Table
	Trees   *skill.Trees
	Zones   *zone.Index
	Routes  route.WalkerRoutes
	NPCs    *npc.Table
	Doors   *door.Table
	Statics *staticobject.Table
	Geo     *engine.Engine
	Finder  *pathfind.Finder
}

type geodata struct {
	Engine        *engine.Engine
	Finder        *pathfind.Finder
	Dir           string
	Type          probe.GeoType
	EngineOptions engine.Options
	Pathfind      pathfind.Options
}

func main() {
	paths := parseGameServerFlags()
	newGameServerApp(paths).Run()
}

func parseGameServerFlags() gameServerPaths {
	var paths gameServerPaths
	flag.StringVar(&paths.ConfigPath, "config", "config/server.properties", "game server properties file")
	flag.StringVar(&paths.LoggingPath, "logging", "config/logging.properties", "logging properties file")
	flag.StringVar(&paths.PlayersConfigPath, "players-config", "config/players.properties", "player properties file")
	flag.StringVar(&paths.HexIDPath, "hexid", "config/hexid.txt", "game server hexid properties file")
	flag.StringVar(&paths.GeoConfigPath, "geo-config", "config/geoengine.properties", "geoengine properties file")
	flag.StringVar(&paths.DataRoot, "data-root", ".", "datapack root containing data/xml")
	flag.StringVar(&paths.LogRoot, "log-root", ".", "root directory for log files")
	flag.Parse()
	return paths
}

func newGameServerApp(paths gameServerPaths) *fx.App {
	return fx.New(
		fx.Supply(paths),
		fx.Provide(
			loadGameServerProperties,
			loadPvPFlagOptions,
			loadHexIDProperties,
			gameServerConfigFromLoadedProperties,
			provideGameServerLogger,
			provideGameServerDatabase,
			loadGameData,
			gamesql.NewCharacterStore,
			gamesql.NewItemStore,
			provideIDAllocator,
			provideRoster,
			providePvPFlags,
			provideWorldState,
			provideGroundItemOptions,
			provideGroundItems,
			provideGameClock,
			provideWalker,
			provideWater,
			provideShadowItems,
			provideDecay,
			provideAttackStance,
			provideWorldObjects,
			provideSpawns,
			provideRespawnTask,
			provideAI,
			provideNpcs,
			network.NewSessionValidator,
			provideLoginLinkState,
			provideGameClientLink,
		),
		fx.Invoke(startPvPFlags, startGroundItems, startGameClock, startWalker, startWater, startShadowItems, startDecay, startAttackStance, startWorldObjects, startRespawnTask, startAI, startNpcs, startNpcPersistence, startGameServer),
	)
}

func loadGameServerProperties(paths gameServerPaths) (*config.Properties, error) {
	return config.LoadFile(paths.ConfigPath)
}

func loadPvPFlagOptions(paths gameServerPaths) (task.PvPFlagOptions, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return task.PvPFlagOptions{}, err
	}
	return task.PvPFlagOptionsFromProperties(props)
}

type hexIDProperties struct {
	Props *config.Properties
}

func loadHexIDProperties(paths gameServerPaths) (hexIDProperties, error) {
	props, err := config.LoadFile(paths.HexIDPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return hexIDProperties{}, nil
		}
		return hexIDProperties{}, err
	}
	return hexIDProperties{Props: props}, nil
}

func gameServerConfigFromLoadedProperties(paths gameServerPaths, serverProps *config.Properties, hexProps hexIDProperties) (gameServerConfig, error) {
	return gameServerConfigFromProperties(paths, serverProps, hexProps.Props)
}

func gameServerConfigFromProperties(paths gameServerPaths, serverProps, hexProps *config.Properties) (gameServerConfig, error) {
	listenPort, err := serverProps.Int("GameserverPort", 7777)
	if err != nil {
		return gameServerConfig{}, err
	}
	loginPort, err := serverProps.Int("LoginPort", 9014)
	if err != nil {
		return gameServerConfig{}, err
	}
	requestID, err := serverProps.Int("RequestServerID", 0)
	if err != nil {
		return gameServerConfig{}, err
	}
	maxPlayers, err := serverProps.Int64("MaximumOnlineUsers", 100)
	if err != nil {
		return gameServerConfig{}, err
	}
	if maxPlayers < math.MinInt32 || maxPlayers > math.MaxInt32 {
		return gameServerConfig{}, fmt.Errorf("MaximumOnlineUsers %d outside int32 range", maxPlayers)
	}

	serverID := requestID
	generated := hexProps == nil
	hexID, err := generatedHexID()
	if err != nil {
		return gameServerConfig{}, err
	}
	if hexProps != nil {
		serverID, err = hexProps.Int("ServerID", requestID)
		if err != nil {
			return gameServerConfig{}, err
		}
		hexID, err = model.ParseHexKey(hexProps.String("HexID", "0"))
		if err != nil {
			return gameServerConfig{}, err
		}
	}

	host := serverProps.String("Hostname", "*")
	statusType := link.ServerTypeAuto
	if serverProps.Bool("ServerGMOnly", false) {
		statusType = link.ServerTypeGMOnly
	}
	showClock := serverProps.Bool("ServerListClock", false)
	showBrackets := serverProps.Bool("ServerListBrackets", false)
	serverListAgeLimit, err := serverProps.Int("ServerListAgeLimit", 0)
	if err != nil {
		return gameServerConfig{}, err
	}
	ageLimit := int32(serverListAgeLimit)
	testServer := serverProps.Bool("TestServer", false)
	pvpServer := serverProps.Bool("PvpServer", true)
	return gameServerConfig{
		ListenAddr: listenAddress(serverProps.String("GameserverHostname", "*"), listenPort),
		LoginAddr:  net.JoinHostPort(serverProps.String("LoginHost", "127.0.0.1"), strconv.Itoa(loginPort)),
		Auth: network.LoginServerAuth{
			ServerID:          serverID,
			AcceptAlternateID: serverProps.Bool("AcceptAlternateID", true),
			HexID:             hexID,
			HostName:          host,
			Port:              uint16(listenPort),
			MaxPlayers:        int32(maxPlayers),
			InitialStatus: link.ServerStatus{
				Status:       &statusType,
				ShowClock:    &showClock,
				ShowBrackets: &showBrackets,
				AgeLimit:     &ageLimit,
				TestServer:   &testServer,
				Pvp:          &pvpServer,
			},
		},
		GeneratedHexID: generated,
		HexIDPath:      paths.HexIDPath,
		Database: db.Config{
			URL:      serverProps.String("URL", "jdbc:mariadb://localhost/acis"),
			Login:    serverProps.String("Login", "root"),
			Password: serverProps.String("Password", ""),
		},
	}, nil
}

func listenAddress(host string, port int) string {
	if host == "*" {
		host = ""
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func generatedHexID() ([]byte, error) {
	key := make([]byte, generatedHexIDSize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate hexid: %w", err)
	}
	return key, nil
}

func writeHexIDFile(path string, serverID int, hexID []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create hexid directory: %w", err)
	}
	data := fmt.Sprintf("#the hexID to auth into login\nServerID=%d\nHexID=%s\n", serverID, model.HexKeyText(hexID))
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		return fmt.Errorf("write hexid file: %w", err)
	}
	return nil
}

func provideGameServerLogger(lc fx.Lifecycle, paths gameServerPaths) (zerolog.Logger, error) {
	props, err := config.LoadFile(paths.LoggingPath)
	if err != nil {
		return zerolog.Logger{}, err
	}
	cfg, err := logging.ConfigFromProperties(props)
	if err != nil {
		return zerolog.Logger{}, err
	}
	rt, err := logging.Setup(paths.LogRoot, cfg, os.Stderr)
	if err != nil {
		return zerolog.Logger{}, err
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { return rt.Close() }})
	return rt.Logger, nil
}

func provideGameServerDatabase(lc fx.Lifecycle, cfg gameServerConfig) (*sql.DB, error) {
	pool, err := db.Open(cfg.Database)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error { return pool.PingContext(ctx) },
		OnStop:  func(context.Context) error { return pool.Close() },
	})
	return pool, nil
}

func loadGameData(paths gameServerPaths, log zerolog.Logger) (*gameData, error) {
	xmlRoot := filepath.Join(paths.DataRoot, "data", "xml")
	players, err := gamexml.LoadPlayerTemplates(filepath.Join(xmlRoot, "classes"))
	if err != nil {
		return nil, err
	}
	levels, err := gamexml.LoadPlayerLevels(filepath.Join(xmlRoot, "playerLevels.xml"))
	if err != nil {
		return nil, err
	}
	items, err := gamexml.LoadItemTemplates(filepath.Join(xmlRoot, "items"))
	if err != nil {
		return nil, err
	}
	skills, err := gamexml.LoadSkillDefinitions(filepath.Join(xmlRoot, "skills"))
	if err != nil {
		return nil, err
	}
	trees, err := gamexml.LoadSkillTrees(filepath.Join(xmlRoot, "skillstrees"))
	if err != nil {
		return nil, err
	}
	zones, err := gamexml.LoadZones(filepath.Join(xmlRoot, "zones"))
	if err != nil {
		return nil, err
	}
	routes, err := gamexml.LoadWalkerRoutes(filepath.Join(xmlRoot, "walkerRoutes.xml"))
	if err != nil {
		return nil, err
	}
	npcs, err := gamexml.LoadNPCTemplates(filepath.Join(xmlRoot, "npcs"), items, log)
	if err != nil {
		return nil, err
	}
	doors, err := gamexml.LoadDoors(filepath.Join(xmlRoot, "doors.xml"))
	if err != nil {
		return nil, err
	}
	statics, err := gamexml.LoadStaticObjects(filepath.Join(xmlRoot, "staticObjects.xml"))
	if err != nil {
		return nil, err
	}
	geo, err := loadGeodata(paths)
	if err != nil {
		return nil, err
	}
	log.Info().Str("geodata_dir", geo.Dir).Str("geodata_type", string(geo.Type)).Int("npc_templates", npcs.Len()).Int("skills", skills.Len()).Msg("game data loaded")
	return &gameData{
		Players: players,
		Levels:  levels,
		Items:   items,
		Skills:  skills,
		Trees:   trees,
		Zones:   zones,
		Routes:  routes,
		NPCs:    npcs,
		Doors:   doors,
		Statics: statics,
		Geo:     geo.Engine,
		Finder:  geo.Finder,
	}, nil
}

func loadGeodata(paths gameServerPaths) (*geodata, error) {
	props, err := config.LoadFile(paths.GeoConfigPath)
	if err != nil {
		return nil, err
	}

	engineOptions, err := engine.OptionsFromProperties(props)
	if err != nil {
		return nil, err
	}
	pathOptions, err := pathfind.OptionsFromProperties(props)
	if err != nil {
		return nil, err
	}

	geo := &geodata{
		Dir:           resolveGeodataDir(paths.DataRoot, props.String("GeoDataPath", "")),
		Type:          probe.GeoType(props.String("GeoDataType", string(probe.L2OFF))),
		EngineOptions: engineOptions,
		Pathfind:      pathOptions,
	}
	geo.Engine, err = probe.LoadEngine(geo.Dir, geo.Type, geo.EngineOptions)
	if err != nil {
		return nil, err
	}
	geo.Finder = pathfind.New(geo.Engine, geo.Pathfind)
	return geo, nil
}

func resolveGeodataDir(dataRoot, configured string) string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return filepath.Join(dataRoot, "data", "geodata")
	}
	if filepath.IsAbs(configured) {
		return configured
	}

	clean := filepath.Clean(configured)
	if clean == "data" || strings.HasPrefix(clean, "data"+string(os.PathSeparator)) {
		return filepath.Join(dataRoot, clean)
	}
	return clean
}

func provideIDAllocator(pool *sql.DB, log zerolog.Logger) (*idfactory.Allocator, error) {
	return idfactory.New(context.Background(), pool, log)
}

func provideRoster(cfg gameServerConfig, data *gameData, characters *gamesql.CharacterStore, items *gamesql.ItemStore, ids *idfactory.Allocator) *manager.Roster {
	return manager.NewRoster(characters, items, data.Players, data.Items, ids, manager.DefaultDeleteAfter, time.Now)
}

func providePvPFlags(opts task.PvPFlagOptions) *task.PvPFlags {
	return task.NewPvPFlags(opts, time.Now)
}

func startPvPFlags(lc fx.Lifecycle, flags *task.PvPFlags, opts task.PvPFlagOptions, log zerolog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			for _, key := range opts.UnsupportedKeys {
				log.Warn().Str("file", "players.properties").Str("key", key).Msg("unsupported Karma/PvP config option")
			}
			return nil
		},
	})
	startTicker(lc, log, flags.Start)
}

// startTicker wires a component's fixed-interval task into the fx
// lifecycle: started once fx starts, stopped once fx stops.
func startTicker(lc fx.Lifecycle, log zerolog.Logger, start func(zerolog.Logger) *scheduler.Ticker) {
	var ticker *scheduler.Ticker
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ticker = start(log)
			return nil
		},
		OnStop: func(context.Context) error {
			if ticker != nil {
				ticker.Stop()
			}
			return nil
		},
	})
}

func provideWorldState() *world.State {
	return world.New()
}

func provideGroundItemOptions(props *config.Properties) (task.GroundItemOptions, error) {
	return task.GroundItemOptionsFromProperties(props)
}

func provideGroundItems(state *world.State, opts task.GroundItemOptions) *task.GroundItems {
	return task.NewGroundItems(state, opts, time.Now)
}

func startGroundItems(lc fx.Lifecycle, items *task.GroundItems, log zerolog.Logger) {
	startTicker(lc, log, items.Start)
}

func provideGameClock() *task.GameClock {
	return task.NewGameClock(time.Now)
}

func startGameClock(lc fx.Lifecycle, clock *task.GameClock, log zerolog.Logger) {
	startTicker(lc, log, func(log zerolog.Logger) *scheduler.Ticker {
		return scheduler.Start(task.GameMinute, clock.Tick, log)
	})
}

func provideWalker(data *gameData) (*task.Walker, error) {
	return task.NewWalker(data.Routes, task.GeoPath{Geo: data.Geo, Finder: data.Finder}, time.Now)
}

func startWalker(lc fx.Lifecycle, walker *task.Walker, log zerolog.Logger) {
	startTicker(lc, log, walker.Start)
}

type gameTaskEffects struct{}

func (gameTaskEffects) GaugeSet(task.WaterActor, time.Duration)  {}
func (gameTaskEffects) Drown(task.WaterActor)                    {}
func (gameTaskEffects) ManaThreshold(int32, *item.Instance, int) {}
func (gameTaskEffects) Expire(int32, *item.Instance)             {}

func provideWater() (*task.Water, error) {
	return task.NewWater(gameTaskEffects{}, time.Now)
}

func startWater(lc fx.Lifecycle, water *task.Water, log zerolog.Logger) {
	startTicker(lc, log, water.Start)
}

func provideShadowItems() (*task.ShadowItems, error) {
	return task.NewShadowItems(gameTaskEffects{})
}

func startShadowItems(lc fx.Lifecycle, items *task.ShadowItems, log zerolog.Logger) {
	startTicker(lc, log, items.Start)
}

// worldDecayEffects removes a decayed actor from the world once its corpse
// display interval elapses, and — once a spawn population has wired its own
// respawn resolution in via SetRespawnHook — arms that actor's next
// respawn. Actors without a decay hook are left alone: nothing outside the
// corpse-decay task itself is expected to register an actor that can't
// decay.
//
// The respawn hook is set after construction (manager.Npcs needs *task.Decay
// itself to register newly spawned actors, so it can't be built first), so
// it's guarded by its own lock separate from anything task.Decay holds.
type worldDecayEffects struct {
	state *world.State

	mu          sync.RWMutex
	respawnHook func(id int32) func()
}

type decayableActor interface {
	Decay(*world.State, func()) bool
}

func (w *worldDecayEffects) Decay(actor task.DecayActor) {
	obj, ok := w.state.Object(actor.ObjectID())
	if !ok {
		return
	}

	var respawn func()
	w.mu.RLock()
	hook := w.respawnHook
	w.mu.RUnlock()
	if hook != nil {
		respawn = hook(actor.ObjectID())
	}

	if d, ok := obj.(decayableActor); ok {
		d.Decay(w.state, respawn)
	}
}

// SetRespawnHook records the callback used to arm a decayed actor's next
// respawn. Call it once, before fx starts the decay ticker.
func (w *worldDecayEffects) SetRespawnHook(f func(id int32) func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.respawnHook = f
}

func provideDecay(state *world.State) (*task.Decay, *worldDecayEffects, error) {
	effects := &worldDecayEffects{state: state}
	d, err := task.NewDecay(effects, time.Now)
	return d, effects, err
}

func startDecay(lc fx.Lifecycle, d *task.Decay, log zerolog.Logger) {
	startTicker(lc, log, d.Start)
}

// npcRespawnEffects re-instantiates one spawn slot once its respawn
// deadline elapses. Like worldDecayEffects, its real resolution
// (manager.Npcs.Respawn) is wired in after construction, since Npcs itself
// needs *task.Respawn to schedule respawns in the first place.
type npcRespawnEffects struct {
	mu   sync.RWMutex
	hook func(key string)
}

func (e *npcRespawnEffects) Respawn(key string) {
	e.mu.RLock()
	hook := e.hook
	e.mu.RUnlock()
	if hook != nil {
		hook(key)
	}
}

// SetHook records the callback that re-instantiates a due spawn slot. Call
// it once, before fx starts the respawn ticker.
func (e *npcRespawnEffects) SetHook(f func(key string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.hook = f
}

func provideRespawnTask() (*task.Respawn, *npcRespawnEffects, error) {
	effects := &npcRespawnEffects{}
	r, err := task.NewRespawn(effects, time.Now)
	return r, effects, err
}

func startRespawnTask(lc fx.Lifecycle, r *task.Respawn, log zerolog.Logger) {
	startTicker(lc, log, r.Start)
}

func provideAI() *task.AI {
	return task.NewAI()
}

func startAI(lc fx.Lifecycle, ai *task.AI, log zerolog.Logger) {
	startTicker(lc, log, ai.Start)
}

// worldAttackStanceEffects stops an actor's attack animation once its
// combat-stance inactivity period elapses. Actors that don't expose a
// physical-attack controller are left alone.
type worldAttackStanceEffects struct{ state *world.State }

type attackStoppableActor interface {
	Stop()
}

func (w worldAttackStanceEffects) AutoAttackStop(actor task.AttackStanceActor) {
	obj, ok := w.state.Object(actor.ObjectID())
	if !ok {
		return
	}
	if s, ok := obj.(attackStoppableActor); ok {
		s.Stop()
	}
}

func provideAttackStance(state *world.State) (*task.AttackStance, error) {
	return task.NewAttackStance(worldAttackStanceEffects{state: state}, time.Now)
}

func startAttackStance(lc fx.Lifecycle, a *task.AttackStance, log zerolog.Logger) {
	startTicker(lc, log, a.Start)
}

// provideWorldObjects spawns every door and static object template into
// state at boot, applying closed doors to geodata immediately.
func provideWorldObjects(data *gameData, ids *idfactory.Allocator, state *world.State) (*manager.WorldObjects, error) {
	return manager.NewWorldObjects(data.Doors, data.Statics, ids, data.Geo, state)
}

func startWorldObjects(objs *manager.WorldObjects, log zerolog.Logger) {
	log.Info().Int("doors", len(objs.Doors())).Int("static_objects", len(objs.StaticObjects())).Msg("world objects spawned")
}

// provideSpawns loads the spawnlist XML and restores dynamic spawn_data
// rows, returning the store alongside so it can be reused to persist state
// back at shutdown.
func provideSpawns(paths gameServerPaths, pool *sql.DB, log zerolog.Logger) (*manager.Spawns, *gamesql.SpawnStore, error) {
	store := gamesql.NewSpawnStore(pool)
	dir := filepath.Join(paths.DataRoot, "data", "xml", "spawnlist")
	spawns, err := manager.LoadSpawns(context.Background(), dir, store)
	if err != nil {
		return nil, nil, err
	}
	log.Info().
		Int("spawn_makers", spawns.Table().MakerCount()).
		Int("spawn_entries", spawns.Table().SpawnCount()).
		Int("persisted_spawn_rows", spawns.StateCount()).
		Msg("spawn list loaded")
	return spawns, store, nil
}

// spawnDropRates are the drop-rate multipliers manager.Npcs applies to kill
// rewards. The server's actual RateDropAdena/RateDropItems/... config
// surface isn't loaded anywhere yet, so every rate is neutral (1x) until
// that's wired up.
var spawnDropRates = item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}

// provideNpcs instantiates every "on start" spawn entry into state at boot,
// then wires the decay/respawn tasks' late-bound hooks to it — manager.Npcs
// needs *task.Decay and *task.Respawn to register actors with, so those
// tasks' own effects can only point back at Npcs after it exists.
func provideNpcs(spawns *manager.Spawns, data *gameData, state *world.State, ids *idfactory.Allocator, decay *task.Decay, decayHooks *worldDecayEffects, respawnTask *task.Respawn, respawnHooks *npcRespawnEffects, ai *task.AI, ground *task.GroundItems, log zerolog.Logger) (*manager.Npcs, error) {
	npcs, err := manager.NewNpcs(spawns, data.NPCs, data.Geo, state, ids, decay, respawnTask, ai, data.Items, ground, spawnDropRates, time.Now, log)
	if err != nil {
		return nil, err
	}
	decayHooks.SetRespawnHook(npcs.RespawnHook)
	respawnHooks.SetHook(npcs.Respawn)
	return npcs, nil
}

func startNpcs(npcs *manager.Npcs, log zerolog.Logger) {
	log.Info().
		Int("live_npcs", npcs.LiveCount()).
		Int("deferred_territory_spawns", npcs.DeferredCount()).
		Int("restored_dead_spawns", npcs.RestoredDeadCount()).
		Int("skipped_non_combat_spawns", npcs.SkippedNonCombatCount()).
		Msg("npc spawns loaded")
}

// startNpcPersistence syncs every live database-tracked spawn's current
// HP/position into its spawn.State row and saves spawn_data at shutdown,
// mirroring the reference server's own save-on-shutdown behavior.
func startNpcPersistence(lc fx.Lifecycle, npcs *manager.Npcs, spawns *manager.Spawns, store *gamesql.SpawnStore, log zerolog.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			npcs.SyncPersistedState()
			if err := spawns.Save(ctx, store); err != nil {
				log.Warn().Err(err).Msg("save spawn data")
			}
			return nil
		},
	})
}

type loginLinkState struct {
	mu   sync.RWMutex
	link *network.LoginLink
}

func provideLoginLinkState() *loginLinkState {
	return &loginLinkState{}
}

func (s *loginLinkState) set(link *network.LoginLink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.link = link
}

func (s *loginLinkState) clear(link *network.LoginLink) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.link == link {
		s.link = nil
	}
}

func (s *loginLinkState) get() *network.LoginLink {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.link
}

func provideGameClientLink(
	data *gameData,
	roster *manager.Roster,
	items *gamesql.ItemStore,
	validator *network.SessionValidator,
	links *loginLinkState,
	state *world.State,
	log zerolog.Logger,
) *network.GameClientLink {
	return network.NewGameClientLink(validator, links.get, roster, items, data.Players, data.Items, state, log)
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

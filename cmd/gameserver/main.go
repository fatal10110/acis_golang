// Command gameserver boots the game server process.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"flag"
	"fmt"
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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
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
	ListenAddr string
	LoginAddr  string
	Auth       network.LoginServerAuth
	Database   db.Config
}

type gameData struct {
	Players *player.TemplateTable
	Levels  *player.LevelTable
	Items   *item.Table
	Geo     *engine.Engine
	Finder  *pathfind.Finder
}

type geodata struct {
	Engine   *engine.Engine
	Finder   *pathfind.Finder
	Dir      string
	Type     probe.GeoType
	Pathfind pathfind.Options
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
			network.NewSessionValidator,
			provideLoginLinkState,
			provideGameClientLink,
		),
		fx.Invoke(startPvPFlags, startGameServer),
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
		if os.IsNotExist(err) {
			return hexIDProperties{}, nil
		}
		return hexIDProperties{}, err
	}
	return hexIDProperties{Props: props}, nil
}

func gameServerConfigFromLoadedProperties(paths gameServerPaths, serverProps *config.Properties, hexProps hexIDProperties) (gameServerConfig, error) {
	return gameServerConfigFromProperties(paths, serverProps, hexProps.Props)
}

func gameServerConfigFromProperties(_ gameServerPaths, serverProps, hexProps *config.Properties) (gameServerConfig, error) {
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
	maxPlayers, err := serverProps.Int("MaximumOnlineUsers", 100)
	if err != nil {
		return gameServerConfig{}, err
	}

	serverID := requestID
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
		},
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
	geo, err := loadGeodata(paths)
	if err != nil {
		return nil, err
	}
	log.Info().Str("geodata_dir", geo.Dir).Str("geodata_type", string(geo.Type)).Msg("game data loaded")
	return &gameData{Players: players, Levels: levels, Items: items, Geo: geo.Engine, Finder: geo.Finder}, nil
}

func loadGeodata(paths gameServerPaths) (*geodata, error) {
	props, err := config.LoadFile(paths.GeoConfigPath)
	if err != nil {
		return nil, err
	}

	options, err := pathfind.OptionsFromProperties(props)
	if err != nil {
		return nil, err
	}

	geo := &geodata{
		Dir:      resolveGeodataDir(paths.DataRoot, props.String("GeoDataPath", "")),
		Type:     probe.GeoType(props.String("GeoDataType", string(probe.L2OFF))),
		Pathfind: options,
	}
	geo.Engine, err = probe.LoadEngine(geo.Dir, geo.Type)
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
	var ticker *scheduler.Ticker
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			for _, key := range opts.UnsupportedKeys {
				log.Warn().Str("file", "players.properties").Str("key", key).Msg("unsupported Karma/PvP config option")
			}
			ticker = flags.Start(log)
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
	log zerolog.Logger,
) *network.GameClientLink {
	return network.NewGameClientLink(validator, links.get, roster, items, data.Players, data.Items, log)
}

func startGameServer(lc fx.Lifecycle, cfg gameServerConfig, _ *gameData, _ *manager.Roster, validator *network.SessionValidator, links *loginLinkState, clients *network.GameClientLink, log zerolog.Logger) {
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
					PlayerAuthResponse: validator.Resolve,
				}, network.DefaultReconnectDelay, func(link *network.LoginLink) {
					links.set(link)
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

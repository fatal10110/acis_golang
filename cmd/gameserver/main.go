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
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/fx"

	"github.com/fatal10110/acis_golang/internal/commons/db"
	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/commons/logging"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
)

const generatedHexIDSize = 16

type gameServerPaths struct {
	ConfigPath  string
	LoggingPath string
	HexIDPath   string
	DataRoot    string
	LogRoot     string
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
}

func main() {
	paths := parseGameServerFlags()
	newGameServerApp(paths).Run()
}

func parseGameServerFlags() gameServerPaths {
	var paths gameServerPaths
	flag.StringVar(&paths.ConfigPath, "config", "config/server.properties", "game server properties file")
	flag.StringVar(&paths.LoggingPath, "logging", "config/logging.properties", "logging properties file")
	flag.StringVar(&paths.HexIDPath, "hexid", "config/hexid.txt", "game server hexid properties file")
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
			loadHexIDProperties,
			gameServerConfigFromLoadedProperties,
			provideGameServerLogger,
			provideGameServerDatabase,
			loadGameData,
			gamesql.NewCharacterStore,
			gamesql.NewItemStore,
			provideIDAllocator,
			provideRoster,
			network.NewSessionValidator,
			provideLoginLinkState,
		),
		fx.Invoke(startGameServer),
	)
}

func loadGameServerProperties(paths gameServerPaths) (*config.Properties, error) {
	return config.LoadFile(paths.ConfigPath)
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
	log.Info().Msg("minimal game data loaded")
	return &gameData{Players: players, Levels: levels, Items: items}, nil
}

func provideIDAllocator(pool *sql.DB, log zerolog.Logger) (*idfactory.Allocator, error) {
	return idfactory.New(context.Background(), pool, log)
}

func provideRoster(cfg gameServerConfig, data *gameData, characters *gamesql.CharacterStore, items *gamesql.ItemStore, ids *idfactory.Allocator) *manager.Roster {
	return manager.NewRoster(characters, items, data.Players, data.Items, ids, manager.DefaultDeleteAfter, time.Now)
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

func startGameServer(lc fx.Lifecycle, cfg gameServerConfig, _ *gameData, _ *manager.Roster, validator *network.SessionValidator, links *loginLinkState, log zerolog.Logger) {
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
				if err := network.Serve(runCtx, ln, func(ctx context.Context, conn *network.Conn) {
					handleGameClient(ctx, conn, log)
				}, log); err != nil {
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

func handleGameClient(_ context.Context, conn *network.Conn, log zerolog.Logger) {
	defer conn.Close()
	log.Warn().Msg("game client dispatcher is not wired yet")
}

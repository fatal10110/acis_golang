// Command loginserver boots the login server process.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/fx"

	"github.com/fatal10110/acis_golang/internal/commons/db"
	"github.com/fatal10110/acis_golang/internal/commons/logging"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/loginserver"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	loginsql "github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
)

type loginServerPaths struct {
	ConfigPath      string
	LoggingPath     string
	ServerNamesPath string
	BannedIPsPath   string
	LogRoot         string
}

type loginServerConfig struct {
	ClientAddr          string
	GameServerAddr      string
	AllowNewGameServers bool
	AutoCreateAccounts  bool
	LoginTryBeforeBan   int
	LoginBlockAfterBan  time.Duration
	Database            db.Config
}

func main() {
	paths := parseLoginServerFlags()
	newLoginServerApp(paths).Run()
}

func parseLoginServerFlags() loginServerPaths {
	var paths loginServerPaths
	flag.StringVar(&paths.ConfigPath, "config", "config/loginserver.properties", "login server properties file")
	flag.StringVar(&paths.LoggingPath, "logging", "config/logging.properties", "logging properties file")
	flag.StringVar(&paths.ServerNamesPath, "server-names", "serverNames.xml", "server id/name list file")
	flag.StringVar(&paths.BannedIPsPath, "banned-ips", "config/banned_ips.properties", "banned IP list file")
	flag.StringVar(&paths.LogRoot, "log-root", ".", "root directory for log files")
	flag.Parse()
	return paths
}

func newLoginServerApp(paths loginServerPaths) *fx.App {
	return fx.New(
		fx.Supply(paths),
		fx.Provide(
			loadLoginServerProperties,
			loginServerConfigFromLoadedProperties,
			provideLoginServerLogger,
			provideLoginServerDatabase,
			loginsql.NewAccountStore,
			loginsql.NewGameServerStore,
			provideServerNames,
			provideServerRegistry,
			manager.NewSessionStore,
			manager.NewRSAKeyPool,
			manager.NewLoginKeyPool,
			provideIPBanList,
			provideGameServerLink,
			provideClientLink,
		),
		fx.Invoke(startLoginServer),
	)
}

func loadLoginServerProperties(paths loginServerPaths) (*config.Properties, error) {
	return config.LoadFile(paths.ConfigPath)
}

func loginServerConfigFromLoadedProperties(paths loginServerPaths, props *config.Properties) (loginServerConfig, error) {
	return loginServerConfigFromProperties(paths, props)
}

func loginServerConfigFromProperties(_ loginServerPaths, props *config.Properties) (loginServerConfig, error) {
	clientPort, err := props.Int("LoginserverPort", 2106)
	if err != nil {
		return loginServerConfig{}, err
	}
	linkPort, err := props.Int("LoginPort", 9014)
	if err != nil {
		return loginServerConfig{}, err
	}
	loginTryBeforeBan, err := props.Int("LoginTryBeforeBan", loginserver.DefaultLoginTryBeforeBan)
	if err != nil {
		return loginServerConfig{}, err
	}
	loginBlockAfterBan, err := props.Int("LoginBlockAfterBan", int(loginserver.DefaultLoginBlockAfterBan/time.Second))
	if err != nil {
		return loginServerConfig{}, err
	}

	return loginServerConfig{
		ClientAddr:          listenAddress(props.String("LoginserverHostname", "*"), clientPort),
		GameServerAddr:      listenAddress(props.String("LoginHostname", "*"), linkPort),
		AllowNewGameServers: props.Bool("AcceptNewGameServer", false),
		AutoCreateAccounts:  props.Bool("AutoCreateAccounts", true),
		LoginTryBeforeBan:   loginTryBeforeBan,
		LoginBlockAfterBan:  time.Duration(loginBlockAfterBan) * time.Second,
		Database: db.Config{
			URL:      props.String("URL", "jdbc:mariadb://localhost/acis"),
			Login:    props.String("Login", "root"),
			Password: props.String("Password", ""),
		},
	}, nil
}

func listenAddress(host string, port int) string {
	if host == "*" {
		host = ""
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func provideLoginServerLogger(lc fx.Lifecycle, paths loginServerPaths) (zerolog.Logger, error) {
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

func provideLoginServerDatabase(lc fx.Lifecycle, cfg loginServerConfig) (*sql.DB, error) {
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

func provideServerNames(paths loginServerPaths) (*manager.ServerNames, error) {
	return manager.LoadServerNames(paths.ServerNamesPath)
}

func provideServerRegistry(lc fx.Lifecycle, store *loginsql.GameServerStore, log zerolog.Logger) *manager.ServerRegistry {
	registry := manager.NewServerRegistry()
	lc.Append(fx.Hook{OnStart: func(ctx context.Context) error {
		servers, err := store.GameServers(ctx)
		if err != nil {
			return err
		}
		known := make(map[int][]byte, len(servers))
		for id, server := range servers {
			known[id] = server.HexID
		}
		registry.Load(known)
		log.Info().Int("registered_gameservers", len(known)).Msg("loginserver registry loaded")
		return nil
	}})
	return registry
}

func provideIPBanList(paths loginServerPaths, log zerolog.Logger) *manager.IPBanList {
	return manager.LoadIPBanList(paths.BannedIPsPath, log)
}

func provideGameServerLink(
	cfg loginServerConfig,
	servers *manager.ServerRegistry,
	names *manager.ServerNames,
	keys *manager.RSAKeyPool,
	sessions *manager.SessionStore,
	bans *manager.IPBanList,
	accounts *loginsql.AccountStore,
	registrations *loginsql.GameServerStore,
	log zerolog.Logger,
) *loginserver.GameServerLink {
	return loginserver.NewGameServerLink(servers, names, keys, sessions, bans, accounts, registrations, cfg.AllowNewGameServers, log)
}

func provideClientLink(
	cfg loginServerConfig,
	accounts *loginsql.AccountStore,
	servers *manager.ServerRegistry,
	sessions *manager.SessionStore,
	bans *manager.IPBanList,
	keys *manager.LoginKeyPool,
	log zerolog.Logger,
) *loginserver.ClientLink {
	return loginserver.NewClientLink(accounts, servers, sessions, bans, keys, cfg.AutoCreateAccounts, cfg.LoginTryBeforeBan, cfg.LoginBlockAfterBan, log)
}

func startLoginServer(lc fx.Lifecycle, cfg loginServerConfig, link *loginserver.GameServerLink, clients *loginserver.ClientLink, log zerolog.Logger) {
	var cancel context.CancelFunc
	var wg sync.WaitGroup

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			runCtx, stop := context.WithCancel(context.Background())
			cancel = stop

			gameLn, err := net.Listen("tcp", cfg.GameServerAddr)
			if err != nil {
				return fmt.Errorf("listen for gameservers on %s: %w", cfg.GameServerAddr, err)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := link.Serve(runCtx, gameLn); err != nil {
					log.Error().Err(err).Str("addr", cfg.GameServerAddr).Msg("gameserver link listener stopped")
				}
			}()
			log.Info().Str("addr", cfg.GameServerAddr).Msg("listening for gameservers")

			clientLn, err := net.Listen("tcp", cfg.ClientAddr)
			if err != nil {
				stop()
				_ = gameLn.Close()
				return fmt.Errorf("listen for login clients on %s: %w", cfg.ClientAddr, err)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := clients.Serve(runCtx, clientLn); err != nil {
					log.Error().Err(err).Str("addr", cfg.ClientAddr).Msg("login client listener stopped")
				}
			}()
			log.Info().Str("addr", cfg.ClientAddr).Msg("listening for login clients")
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

package main

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/shutdown"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// restartExitCode is the exit status of an admin-requested restart; an
// admin-requested shutdown exits 0. A supervisor restarts the server on
// the former only.
const restartExitCode = 2

// provideLoginStatus returns the status the login server lists the server
// under, starting from server.properties' ServerGMOnly and
// MaximumOnlineUsers and pushed over the current login link.
func provideLoginStatus(cfg gameServerConfig, links *loginLinkState, log zerolog.Logger) *network.LoginStatus {
	initial := cfg.Auth.InitialStatus.Status
	gmOnly := initial != nil && *initial == link.ServerTypeGMOnly
	return network.NewLoginStatus(gmOnly, cfg.Auth.MaxPlayers, func(status link.ServerStatus) {
		l := links.get()
		if l == nil {
			return
		}
		if err := l.SendServerStatus(status); err != nil {
			log.Error().Err(err).Msg("send ServerStatus to loginserver")
		}
	})
}

// provideShutdown returns the timed shutdown //server runs, warning every
// player in state and ending the process through stopProcess.
func provideShutdown(pool *sim.Pool, state *world.State, status *network.LoginStatus, shutdowner fx.Shutdowner, log zerolog.Logger) *shutdown.Countdown {
	return shutdown.New(pool.NewQueue("shutdown"), network.NewShutdownNotices(state), status, stopProcess(shutdowner, log), log)
}

// stopProcess ends the process through the same stop hooks as SIGTERM,
// exiting with restartExitCode for a restart.
func stopProcess(shutdowner fx.Shutdowner, log zerolog.Logger) func(restart bool) {
	return func(restart bool) {
		code := 0
		if restart {
			code = restartExitCode
		}
		if err := shutdowner.Shutdown(fx.ExitCode(code)); err != nil {
			log.Error().Err(err).Msg("admin shutdown")
		}
	}
}

package main

import (
	"context"
	"database/sql"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// serverSession keeps server_memo's crash flag: set once boot completes,
// cleared at the very end of a clean shutdown, so a boot that finds it still
// set knows the previous session crashed.
//
// marked is written by markServerSession's start hook and read by
// clearServerSessionOnStop's stop hook; fx runs its hooks one at a time.
type serverSession struct {
	store  *gamesql.ServerCrashStore
	marked bool
}

func provideServerSession(db *sql.DB) *serverSession {
	return &serverSession{store: gamesql.NewServerCrashStore(db)}
}

// clearServerSessionOnStop registers the stop hook that records a clean
// shutdown. It is registered before every hook that saves, so, stop hooks
// running in reverse, it runs after all of them, the last write before the
// database closes: a shutdown cut off in its saves leaves the flag set. A
// boot that failed before markServerSession ran leaves the flag as the
// previous session left it.
func clearServerSessionOnStop(lc fx.Lifecycle, s *serverSession, log zerolog.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			if !s.marked {
				return nil
			}
			saveOnStop(ctx, shutdownSaveTimeout, log, "clear server crash flag", s.store.MarkStopped)
			return nil
		},
	})
}

// markServerSession, the last start hook before the game listener opens,
// logs a crash of the previous session or else sets the crash flag for this
// one. A database error is logged and does not fail boot.
func markServerSession(lc fx.Lifecycle, s *serverSession, log zerolog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			s.marked = true
			crashed, err := s.store.MarkRunning(ctx)
			switch {
			case err != nil:
				log.Error().Err(err).Msg("server crash flag")
			case crashed:
				log.Info().Msg("Server crashed on last session!")
			}
			return nil
		},
	})
}

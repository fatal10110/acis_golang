package main

import (
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/rs/zerolog"
)

// communityBoard is the community board's settings and, while the board
// is on, its mail; ShowServerNews is the server news page shown at login.
type communityBoard struct {
	Config         bbs.Config
	Mailbox        *bbs.Mailbox
	ShowServerNews bool
}

// loadBoardConfig reads the server.properties community board and server
// news settings, defaulting as shipped.
func loadBoardConfig(paths gameServerPaths) (bbs.Config, bool, error) {
	props, err := config.LoadFile(paths.ConfigPath)
	if err != nil {
		return bbs.Config{}, false, err
	}
	f := config.NewFields(props, "community board")
	def := bbs.DefaultConfig()
	cfg := bbs.Config{
		Enabled: f.Bool("EnableCommunityBoard", def.Enabled),
		Home:    f.String("BBSDefault", def.Home),
	}
	news := f.Bool("ShowServerNews", false)
	return cfg, news, f.Err()
}

// provideCommunityBoard loads the board settings and, when the board is
// on, every stored mail, after the id factory has dropped the mail of
// characters that no longer exist. Mail writes go through the persistence
// worker.
func provideCommunityBoard(ctx bootContext, paths gameServerPaths, pool *sql.DB, _ *idfactory.Allocator, worker *persist.Worker, log zerolog.Logger) (communityBoard, error) {
	cfg, news, err := loadBoardConfig(paths)
	if err != nil {
		return communityBoard{}, err
	}
	board := communityBoard{Config: cfg, ShowServerNews: news}
	if !cfg.Enabled {
		return board, nil
	}
	store := gamesql.NewMailStore(pool)
	mails, skipped, err := store.Load(ctx)
	if err != nil {
		return communityBoard{}, fmt.Errorf("restore mail: %w", err)
	}
	if skipped > 0 {
		log.Warn().Int("skipped", skipped).Msg("mail rows with an unknown folder or no send date skipped")
	}
	log.Info().Int("mails", len(mails)).Msg("mail loaded")
	board.Mailbox = bbs.NewMailbox(store, worker, log)
	board.Mailbox.Restore(mails)
	return board, nil
}

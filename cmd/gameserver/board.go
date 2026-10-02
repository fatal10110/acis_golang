package main

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/rs/zerolog"
)

// communityBoard is the community board's settings and, while the board
// is on, its mail, forums and favorites; ShowServerNews is the server news
// page shown at login.
type communityBoard struct {
	Config         bbs.Config
	Mailbox        *bbs.Mailbox
	Forums         *bbs.Forums
	Favorites      *bbs.Favorites
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
// on, every stored mail, forum and favorite, after the id factory has
// dropped the rows of characters and clans that no longer exist, then
// gives every clan at level 2 or more its clan forums. Board writes go
// through the persistence worker.
func provideCommunityBoard(ctx bootContext, paths gameServerPaths, pool *sql.DB, _ *idfactory.Allocator, clans *clan.Service, worker *persist.Worker, log zerolog.Logger) (communityBoard, error) {
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

	if board.Forums, err = restoreForums(ctx, pool, clans, worker, log); err != nil {
		return communityBoard{}, err
	}
	favStore := gamesql.NewFavoriteStore(pool)
	favs, err := favStore.Load(ctx)
	if err != nil {
		return communityBoard{}, fmt.Errorf("restore favorites: %w", err)
	}
	board.Favorites = bbs.NewFavorites(favStore, worker, log)
	board.Favorites.Restore(favs)
	return board, nil
}

// restoreForums loads the stored forums, then gives each clan at level 2
// or more, in ascending id order, the clan forums it lacks.
func restoreForums(ctx context.Context, pool *sql.DB, clans *clan.Service, worker *persist.Worker, log zerolog.Logger) (*bbs.Forums, error) {
	store := gamesql.NewForumStore(pool)
	forumRows, topics, posts, err := store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("restore forums: %w", err)
	}
	forums := bbs.NewForums(store, worker, log)
	if n, stopped := forums.Restore(forumRows, topics, posts); stopped {
		log.Error().Int("forums", n).Msg("forums: loading stopped at a row that does not read or belongs to no loaded forum or topic")
	} else {
		log.Info().Int("forums", n).Msg("forums loaded")
	}
	all := clans.Table().Clans()
	slices.SortFunc(all, func(a, b *clan.Clan) int { return cmp.Compare(a.ID(), b.ID()) })
	for _, cl := range all {
		forums.EnsureClanForums(cl.ID(), cl.Level())
	}
	return forums, nil
}

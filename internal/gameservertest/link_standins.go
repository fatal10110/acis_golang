package gameservertest

import (
	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/announcement"
	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/fence"
	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/lottery"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/relation"
	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
)

// fillLinkStandIns gives the link an in-memory service with no store for
// each one the Boot options left out (the board's forums and favorites
// unless WithCommunityBoard enables the board, the scheme buffer, lottery
// and fishing championship without their options), and fences placed on an
// empty geodata engine. The link requires every one.
func fillLinkStandIns(cfg *network.GameClientLinkConfig, q *queues, log zerolog.Logger) {
	if cfg.Relations == nil {
		cfg.Relations = relation.NewManager(nil)
	}
	if cfg.Petitions == nil {
		cfg.Petitions = petition.NewManager(petition.DefaultConfig(), nil, nil, nil, nil)
	}
	if cfg.Forums == nil {
		cfg.Forums = bbs.NewForums(nil, nil, log)
	}
	if cfg.Favorites == nil {
		cfg.Favorites = bbs.NewFavorites(nil, nil, log)
	}
	if cfg.SchemeBuffer == nil {
		cfg.SchemeBuffer = schemebuffer.New(schemebuffer.DefaultConfig(), nil, nil)
	}
	if cfg.Wedding == nil {
		ids, _ := cfg.IDs.(wedding.IDs)
		cfg.Wedding = wedding.NewManager(wedding.DefaultConfig(), ids, nil, cfg.TradeClock)
	}
	if cfg.FishingChampionship == nil {
		cfg.FishingChampionship = fishchamp.New(fishchamp.Config{}, nil, nil, q.NewQueue("fishchamp"), log)
	}
	if cfg.Lottery == nil {
		cfg.Lottery = lottery.New(lottery.DefaultConfig(), nil, nil, nil, q.NewQueue("lottery"), log)
	}
	if cfg.Announcements == nil {
		cfg.Announcements = announcement.NewRegistry(nil, network.NewAnnouncer(cfg.World), log, q.NewQueue("announcements"))
	}
	if cfg.Fences == nil {
		cfg.Fences = fence.NewManager(engine.New(), cfg.World, cfg.IDs)
	}
	if cfg.Clans == nil {
		cfg.Clans = clan.NewService(nil, nil, nil, cfg.IDs, clan.DefaultConfig(), nil, log)
	}
}

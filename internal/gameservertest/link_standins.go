package gameservertest

import (
	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/fence"
	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/lottery"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
)

// fillLinkStandIns gives the link an in-memory service with no store for
// each one a Boot option left out: the board's forums and favorites unless
// WithCommunityBoard enables the board, the lottery and fishing championship without their options, and
// fences placed on an empty geodata engine. The link requires every one.
func fillLinkStandIns(cfg *network.GameClientLinkConfig, q *queues, log zerolog.Logger) {
	if cfg.Forums == nil {
		cfg.Forums = bbs.NewForums(nil, nil, log)
	}
	if cfg.Favorites == nil {
		cfg.Favorites = bbs.NewFavorites(nil, nil, log)
	}
	if cfg.FishingChampionship == nil {
		cfg.FishingChampionship = fishchamp.New(fishchamp.Config{}, nil, nil, q.NewQueue("fishchamp"), log)
	}
	if cfg.Lottery == nil {
		cfg.Lottery = lottery.New(lottery.DefaultConfig(), nil, nil, nil, q.NewQueue("lottery"), log)
	}
	if cfg.Fences == nil {
		cfg.Fences = fence.NewManager(engine.New(), cfg.World, cfg.IDs)
	}
}

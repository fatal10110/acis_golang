package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/announcement"
	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/fence"
	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/lottery"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/relation"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
)

// newBareGameClientLink builds a link through NewGameClientLink from cfg,
// filling in the queues and effect activity the constructor requires when
// cfg leaves them out, and an in-memory stand-in with no store for each
// required domain service cfg leaves out.
func newBareGameClientLink(t *testing.T, cfg GameClientLinkConfig) *GameClientLink {
	t.Helper()
	if cfg.Queues == nil {
		cfg.Queues = testQueues(t)
	}
	if cfg.Effects.Activity == nil {
		cfg.Effects.Activity = task.NewEffects()
	}
	if cfg.Relations == nil {
		cfg.Relations = relation.NewManager(nil)
	}
	if cfg.Petitions == nil {
		cfg.Petitions = petition.NewManager(petition.DefaultConfig(), nil, nil, nil, nil)
	}
	if cfg.Forums == nil {
		cfg.Forums = bbs.NewForums(nil, nil, cfg.Log)
	}
	if cfg.Favorites == nil {
		cfg.Favorites = bbs.NewFavorites(nil, nil, cfg.Log)
	}
	if cfg.SchemeBuffer == nil {
		cfg.SchemeBuffer = schemebuffer.New(schemebuffer.DefaultConfig(), nil, nil)
	}
	if cfg.Wedding == nil {
		ids, _ := cfg.IDs.(wedding.IDs)
		cfg.Wedding = wedding.NewManager(wedding.DefaultConfig(), ids, nil, cfg.TradeClock)
	}
	if cfg.FishingChampionship == nil {
		cfg.FishingChampionship = fishchamp.New(fishchamp.Config{}, nil, nil, cfg.Queues.NewQueue("fishchamp"), cfg.Log)
	}
	if cfg.Lottery == nil {
		cfg.Lottery = lottery.New(lottery.DefaultConfig(), nil, nil, nil, cfg.Queues.NewQueue("lottery"), cfg.Log)
	}
	if cfg.Announcements == nil {
		cfg.Announcements = announcement.NewRegistry(nil, NewAnnouncer(cfg.World), cfg.Log, cfg.Queues.NewQueue("announcements"))
	}
	if cfg.Fences == nil {
		cfg.Fences = fence.NewManager(engine.New(), cfg.World, cfg.IDs)
	}
	if cfg.Clans == nil {
		cfg.Clans = clan.NewService(nil, nil, nil, cfg.IDs, clan.DefaultConfig(), nil, cfg.Log)
	}
	link, err := NewGameClientLink(cfg)
	if err != nil {
		t.Fatalf("new game client link: %v", err)
	}
	return link
}

// TestNewGameClientLinkRequiresServices pins that the link builds no
// stand-in of its own: a config leaving out a domain service is refused,
// naming it.
func TestNewGameClientLinkRequiresServices(t *testing.T) {
	cfg := GameClientLinkConfig{Queues: testQueues(t), Effects: effect.Env{Activity: task.NewEffects()}}
	_, err := NewGameClientLink(cfg)
	if err == nil || err.Error() != "network: GameClientLinkConfig.Relations is required" {
		t.Fatalf("NewGameClientLink() error = %v, want missing Relations", err)
	}
}

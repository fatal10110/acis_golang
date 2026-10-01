package main

import (
	"context"
	"path/filepath"

	"github.com/fatal10110/acis_golang/internal/gameserver/announcement"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// provideAnnouncements returns the server announcements, kept in the
// datapack's announcements.xml, scheduled on pool and delivered to every
// player in state.
func provideAnnouncements(paths gameServerPaths, pool *sim.Pool, state *world.State, log zerolog.Logger) *announcement.Registry {
	file := gamexml.AnnouncementFile{Path: filepath.Join(paths.DataRoot, "data", "xml", "announcements.xml"), Log: log}
	return announcement.NewRegistry(file, network.NewAnnouncer(state), log, pool.NewQueue("announcements"))
}

// startAnnouncements loads the announcements, starting the automatic
// ones' schedules, and stops every schedule on shutdown.
func startAnnouncements(lc fx.Lifecycle, announcements *announcement.Registry) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return announcements.Load()
		},
		OnStop: func(context.Context) error {
			announcements.Stop()
			return nil
		},
	})
}

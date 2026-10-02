package gameservertest

import (
	"os"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/announcement"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// bootAnnouncements writes file (a file holding none when empty) to path
// and loads it as the server announcements, scheduled on queue and
// delivered to every player in state; their schedules stop when the test
// ends.
func bootAnnouncements(t *testing.T, path, file string, queue *sim.Queue, state *world.State, log zerolog.Logger) *announcement.Registry {
	t.Helper()
	if file == "" {
		file = "<list>\n</list>"
	}
	if err := os.WriteFile(path, []byte(file), 0o644); err != nil {
		t.Fatalf("gameservertest: write announcements: %v", err)
	}
	registry := announcement.NewRegistry(gamexml.AnnouncementFile{Path: path, Log: log}, network.NewAnnouncer(state), log, queue)
	if err := registry.Load(); err != nil {
		t.Fatalf("gameservertest: load announcements: %v", err)
	}
	t.Cleanup(registry.Stop)
	return registry
}

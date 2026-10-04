package main

import (
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/signspriest"
)

// loadSignsPriestConfig reads the events.properties Seven Signs settings
// the priests' dialog reads, each defaulting to its shipped value when the
// key is missing. A malformed value fails boot.
func loadSignsPriestConfig(paths gameServerPaths) (signspriest.Config, error) {
	props, err := config.LoadFile(paths.EventsConfigPath)
	if err != nil {
		return signspriest.Config{}, err
	}
	f := config.NewFields(props, "events")
	def := signspriest.DefaultConfig()
	cfg := signspriest.Config{
		MaxPlayerContrib:    f.Int("MaxPlayerContrib", def.MaxPlayerContrib),
		BypassPrerequisites: f.Bool("SevenSignsBypassPrerequisites", def.BypassPrerequisites),
	}
	return cfg, f.Err()
}

package main

import (
	"database/sql"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/rs/zerolog"
)

// loadFestivalConfig reads the events.properties Festival of Darkness
// schedule settings, in milliseconds, defaulting as the reference does
// when a key is missing.
func loadFestivalConfig(paths gameServerPaths) (festival.Config, error) {
	props, err := config.LoadFile(paths.EventsConfigPath)
	if err != nil {
		return festival.Config{}, err
	}
	f := config.NewFields(props, "events")
	def := festival.DefaultConfig()
	millis := func(key string, d time.Duration) time.Duration {
		return time.Duration(f.Int64(key, d.Milliseconds())) * time.Millisecond
	}
	cfg := festival.Config{
		ManagerStart: millis("FestivalManagerStart", def.ManagerStart),
		Length:       millis("FestivalLength", def.Length),
		CycleLength:  millis("FestivalCycleLength", def.CycleLength),
	}
	return cfg, f.Err()
}

// provideFestival returns the Festival of Darkness, persisting through the
// gameserver database and following the Seven Signs calendar.
func provideFestival(paths gameServerPaths, db *sql.DB, sevenSigns *sevensigns.State, log zerolog.Logger) (*festival.Manager, error) {
	cfg, err := loadFestivalConfig(paths)
	if err != nil {
		return nil, err
	}
	return festival.New(cfg, gamesql.NewFestivalStore(db), sevenSigns, log, time.Now, nil), nil
}

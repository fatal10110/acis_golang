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
// settings, the times in milliseconds, with the shipped defaults for a
// missing key; the party minimum is held to 2 to 9.
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
		MinPlayers:   min(max(f.Int("FestivalMinPlayer", def.MinPlayers), 2), 9),
		FirstSpawn:   millis("FestivalFirstSpawn", def.FirstSpawn),
		FirstSwarm:   millis("FestivalFirstSwarm", def.FirstSwarm),
		SecondSpawn:  millis("FestivalSecondSpawn", def.SecondSpawn),
		SecondSwarm:  millis("FestivalSecondSwarm", def.SecondSwarm),
		ChestSpawn:   millis("FestivalChestSpawn", def.ChestSpawn),
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

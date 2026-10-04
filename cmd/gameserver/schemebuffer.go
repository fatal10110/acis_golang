package main

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/fatal10110/acis_golang/internal/config"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// loadSchemeBufferConfig reads the npcs.properties BufferMaxSchemesPerChar
// (default 4) and BufferStaticCostPerBuff (default -1) keys. A malformed
// value fails boot.
func loadSchemeBufferConfig(paths gameServerPaths) (schemebuffer.Config, error) {
	props, err := config.LoadFile(paths.NpcsConfigPath)
	if err != nil {
		return schemebuffer.Config{}, err
	}
	def := schemebuffer.DefaultConfig()
	f := config.NewFields(props, "scheme buffer")
	cfg := schemebuffer.Config{
		MaxSchemes: f.Int("BufferMaxSchemesPerChar", def.MaxSchemes),
		StaticCost: f.Int("BufferStaticCostPerBuff", def.StaticCost),
	}
	if err := f.Err(); err != nil {
		return schemebuffer.Config{}, err
	}
	return cfg, nil
}

// provideSchemeBuffer returns the scheme buffer offering the buffs of the
// datapack's bufferSkills.xml. The players' schemes are restored by
// startSchemeBuffer.
func provideSchemeBuffer(paths gameServerPaths, data *gameData, log zerolog.Logger) (*schemebuffer.Manager, error) {
	cfg, err := loadSchemeBufferConfig(paths)
	if err != nil {
		return nil, err
	}
	buffs, err := gamexml.LoadBufferSkills(filepath.Join(paths.DataRoot, "data", "xml", "bufferSkills.xml"), data.Skills)
	if err != nil {
		return nil, err
	}
	log.Info().Int("count", buffs.Count()).Msg("scheme buffer: loaded available buffs")
	return schemebuffer.New(cfg, buffs, data.Skills), nil
}

// startSchemeBuffer restores the players' schemes before any character can
// log in, and stores them all once the game server has let every player
// go: the schemes are written at shutdown only.
func startSchemeBuffer(lc fx.Lifecycle, buffer *schemebuffer.Manager, db *sql.DB, log zerolog.Logger) {
	store := gamesql.NewBufferSchemeStore(db)
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			rows, err := store.Load(ctx)
			if err != nil {
				return err
			}
			buffer.Restore(rows, log)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			saveOnStop(ctx, shutdownSaveTimeout, log, "save buffer schemes", func(ctx context.Context) error {
				return store.Save(ctx, buffer.Rows())
			})
			return nil
		},
	})
}

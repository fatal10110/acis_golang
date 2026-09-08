package main

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// failingComponent's provider always errors, forcing the fx graph to fail
// after provideGroundItems has already run.
type failingComponent struct{}

// TestGroundItemsSurviveConstructorFailureAfterRestore drives
// provideGroundItems and startGroundItems through a real fx app whose graph
// has a constructor that fails after ground items have already been
// restored — mirroring provideSpawns or the id scan running out of the
// shared bootContext budget. Before this fix, provideGroundItems cleared
// items_on_ground itself, so a later constructor failure lost every
// restored row permanently: fx never runs OnStart/OnStop hooks once a
// constructor fails (app.err short-circuits both Start and Stop), so
// nothing ever wrote the rows back. The fix defers the clear to an OnStart
// hook, which fx only reaches once every constructor in the graph has
// already succeeded.
func TestGroundItemsSurviveConstructorFailureAfterRestore(t *testing.T) {
	db := sqltest.SharedDB(t)
	if _, err := db.Exec("DELETE FROM items_on_ground"); err != nil {
		t.Fatalf("clear items_on_ground: %v", err)
	}
	if _, err := db.Exec(
		"INSERT INTO items_on_ground (object_id,item_id,count,enchant_level,x,y,z,time) VALUES (?,?,?,?,?,?,?,?)",
		int32(0x20000001), int32(57), int32(1), int32(0), 0, 0, 0, 0,
	); err != nil {
		t.Fatalf("seed ground item: %v", err)
	}

	data := &gameData{Items: item.NewTable([]*item.Template{{ID: 57, Kind: item.KindEtcItem}})}

	failedApp := fx.New(
		fx.NopLogger,
		fx.Supply(data, task.GroundItemOptions{}),
		fx.Provide(
			provideBootContext,
			func() zerolog.Logger { return zerolog.Nop() },
			func() *world.State { return world.New() },
			func() *sql.DB { return db },
			provideGroundItems,
			func() (*failingComponent, error) {
				return nil, errors.New("boot: simulated later-constructor failure")
			},
		),
		fx.Invoke(startGroundItems, func(*failingComponent) {}),
	)

	startCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := failedApp.Start(startCtx); err == nil {
		t.Fatal("app.Start() error = nil, want the simulated constructor failure")
	}
	_ = failedApp.Stop(context.Background())

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM items_on_ground").Scan(&count); err != nil {
		t.Fatalf("count items_on_ground: %v", err)
	}
	if count != 1 {
		t.Fatalf("items_on_ground row count after failed boot = %d, want 1 (restored row must survive a later constructor failure)", count)
	}

	// Sanity check the other direction: a graph that succeeds does clear
	// the table, so restored rows aren't written back twice next boot.
	okApp := fx.New(
		fx.NopLogger,
		fx.Supply(data, task.GroundItemOptions{}),
		fx.Provide(
			provideBootContext,
			func() zerolog.Logger { return zerolog.Nop() },
			func() *world.State { return world.New() },
			func() *sql.DB { return db },
			provideGroundItems,
		),
		fx.Invoke(startGroundItems),
	)
	if err := okApp.Start(startCtx); err != nil {
		t.Fatalf("okApp.Start() error = %v, want nil", err)
	}
	_ = okApp.Stop(context.Background())

	if err := db.QueryRow("SELECT COUNT(*) FROM items_on_ground").Scan(&count); err != nil {
		t.Fatalf("count items_on_ground after successful boot: %v", err)
	}
	if count != 0 {
		t.Fatalf("items_on_ground row count after successful boot = %d, want 0", count)
	}
}

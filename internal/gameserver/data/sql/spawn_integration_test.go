//go:build integration

package sql

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

func TestSpawnStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := NewSpawnStore(sqltest.NewDB(t))

	initial, err := store.LoadStates(ctx)
	if err != nil {
		t.Fatalf("LoadStates() unexpected error: %v", err)
	}
	if len(initial) != 0 {
		t.Fatalf("initial LoadStates() = %v, want empty", initial)
	}

	states := map[string]*spawn.State{
		"alive": {
			Name:        "alive",
			Status:      spawn.StatusAlive,
			CurrentHP:   120,
			CurrentMP:   30,
			Location:    location.Location{X: 1, Y: 2, Z: 3},
			Heading:     4,
			DBValue:     5,
			RespawnTime: 6,
		},
		"dead": {
			Name:        "dead",
			Status:      spawn.StatusDead,
			RespawnTime: 9_000,
		},
		"new": spawn.NewState("new"),
	}
	if err := store.SaveStates(ctx, states); err != nil {
		t.Fatalf("SaveStates() unexpected error: %v", err)
	}

	reloaded, err := NewSpawnStore(store.db).LoadStates(ctx)
	if err != nil {
		t.Fatalf("reloaded LoadStates() unexpected error: %v", err)
	}
	if _, ok := reloaded["new"]; ok {
		t.Fatal("uninitialized state was saved")
	}
	alive := reloaded["alive"]
	if alive == nil || alive.Status != spawn.StatusAlive || alive.CurrentHP != 120 || alive.CurrentMP != 30 ||
		alive.Location != (location.Location{X: 1, Y: 2, Z: 3}) || alive.Heading != 4 || alive.DBValue != 5 || alive.RespawnTime != 6 {
		t.Fatalf("alive row = %+v", alive)
	}
	dead := reloaded["dead"]
	if dead == nil || dead.Status != spawn.StatusDead || dead.RespawnTime != 9_000 {
		t.Fatalf("dead row = %+v", dead)
	}

	alive.CurrentHP = 77
	alive.DBValue = 8
	if err := store.SaveStates(ctx, map[string]*spawn.State{"alive": alive}); err != nil {
		t.Fatalf("update SaveStates() unexpected error: %v", err)
	}
	updated, err := NewSpawnStore(store.db).LoadStates(ctx)
	if err != nil {
		t.Fatalf("updated LoadStates() unexpected error: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("updated rows = %v, want only alive", updated)
	}
	if updated["alive"].CurrentHP != 77 || updated["alive"].DBValue != 8 {
		t.Fatalf("updated alive row = %+v", updated["alive"])
	}
}

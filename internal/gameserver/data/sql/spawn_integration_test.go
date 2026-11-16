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
	store := NewSpawnStore(sqltest.SharedDB(t))

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

// A save with one row the table refuses, or one with no name, commits
// nothing: the rows saved before stay as they were (owner-approved
// divergence, #4685).
func TestSpawnStoreSaveStatesCommitsNothingWhenOneRowFails(t *testing.T) {
	for name, bad := range map[string]*spawn.State{
		"unnamed":          {Status: spawn.StatusAlive},
		"refused_by_table": {Name: "refused", Status: spawn.StatusAlive, CurrentHP: -1},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			store := NewSpawnStore(sqltest.SharedDB(t))
			if err := store.SaveStates(ctx, map[string]*spawn.State{
				"kept": {Name: "kept", Status: spawn.StatusAlive, CurrentHP: 100},
			}); err != nil {
				t.Fatalf("seed SaveStates() unexpected error: %v", err)
			}

			err := store.SaveStates(ctx, map[string]*spawn.State{
				"kept":   {Name: "kept", Status: spawn.StatusAlive, CurrentHP: 120},
				"other":  {Name: "other", Status: spawn.StatusAlive, CurrentHP: 50},
				bad.Name: bad,
			})
			if err == nil {
				t.Fatal("SaveStates() error = nil, want the failed row's error")
			}

			states, err := store.LoadStates(ctx)
			if err != nil {
				t.Fatalf("LoadStates() unexpected error: %v", err)
			}
			if len(states) != 1 || states["kept"] == nil || states["kept"].CurrentHP != 100 {
				t.Fatalf("rows after the failed save = %v, want only kept with CurrentHP 100", states)
			}
		})
	}
}

// SaveState replaces one row and leaves the others; an uninitialized row
// is removed, as a whole-table save would leave it out.
func TestSpawnStoreSaveStateWritesOneRow(t *testing.T) {
	ctx := context.Background()
	store := NewSpawnStore(sqltest.SharedDB(t))
	if err := store.SaveStates(ctx, map[string]*spawn.State{
		"boss":  {Name: "boss", Status: spawn.StatusAlive, CurrentHP: 100},
		"other": {Name: "other", Status: spawn.StatusAlive, CurrentHP: 50},
	}); err != nil {
		t.Fatalf("seed SaveStates() unexpected error: %v", err)
	}

	dead := spawn.State{Name: "boss", Status: spawn.StatusDead, DBValue: 2, RespawnTime: 12345}
	if err := store.SaveState(ctx, dead); err != nil {
		t.Fatalf("SaveState(dead) unexpected error: %v", err)
	}
	if err := store.SaveState(ctx, spawn.State{Name: "fresh", Status: spawn.StatusAlive, CurrentHP: 7}); err != nil {
		t.Fatalf("SaveState(fresh) unexpected error: %v", err)
	}
	states, err := store.LoadStates(ctx)
	if err != nil {
		t.Fatalf("LoadStates() unexpected error: %v", err)
	}
	if got := states["boss"]; got == nil || *got != dead {
		t.Fatalf("boss row = %+v, want %+v", got, dead)
	}
	if got := states["other"]; got == nil || got.CurrentHP != 50 {
		t.Fatalf("other row = %+v, want untouched with CurrentHP 50", got)
	}
	if got := states["fresh"]; got == nil || got.CurrentHP != 7 {
		t.Fatalf("fresh row = %+v, want inserted with CurrentHP 7", got)
	}

	if err := store.SaveState(ctx, spawn.State{Name: "boss", Status: spawn.StatusUninitialized}); err != nil {
		t.Fatalf("SaveState(uninitialized) unexpected error: %v", err)
	}
	if states, err = store.LoadStates(ctx); err != nil {
		t.Fatalf("LoadStates() unexpected error: %v", err)
	}
	if _, ok := states["boss"]; ok || len(states) != 2 {
		t.Fatalf("rows after the uninitialized save = %v, want other and fresh", states)
	}
}

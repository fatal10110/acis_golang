package spawn

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestStateLifecycle(t *testing.T) {
	now := time.UnixMilli(1_000)
	state := NewState("boss_1")

	if state.Status != StatusUninitialized {
		t.Fatalf("new state status = %d, want %d", state.Status, StatusUninitialized)
	}

	loc := location.Location{X: 10, Y: 20, Z: 30}
	if kept := state.CheckAlive(loc, 40, 500, 200, now); kept {
		t.Fatal("CheckAlive() for uninitialized state = true, want false")
	}
	if state.Status != StatusAlive || state.CurrentHP != 500 || state.CurrentMP != 200 || state.Location != loc || state.Heading != 40 || state.RespawnTime != 0 {
		t.Fatalf("state after CheckAlive() = %+v", state)
	}

	state.SetRespawn(2*time.Second, now)
	if state.Status != StatusDead || state.CurrentHP != 0 || state.CurrentMP != 0 || state.Location != (location.Location{}) || state.Heading != 0 {
		t.Fatalf("state after SetRespawn() = %+v", state)
	}
	if state.RespawnTime != 3_000 {
		t.Fatalf("RespawnTime = %d, want 3000", state.RespawnTime)
	}
	if !state.Dead(now.Add(1999 * time.Millisecond)) {
		t.Fatal("Dead() before respawn time = false, want true")
	}
	if state.Dead(now.Add(2 * time.Second)) {
		t.Fatal("Dead() at respawn time = true, want false")
	}

	state.CancelRespawn()
	if state.RespawnTime != 1 {
		t.Fatalf("CancelRespawn() respawn time = %d, want 1", state.RespawnTime)
	}
}

func TestStateCheckAliveRestoresExistingRow(t *testing.T) {
	now := time.UnixMilli(1_000)
	state := &State{
		Name:      "queen_ant",
		Status:    StatusAlive,
		CurrentHP: 123,
		CurrentMP: 45,
		Location:  location.Location{X: 1, Y: 2, Z: 3},
		Heading:   4,
	}

	if kept := state.CheckAlive(location.Location{X: 10, Y: 20, Z: 30}, 40, 500, 200, now); !kept {
		t.Fatal("CheckAlive() for persisted alive state = false, want true")
	}
	if state.CurrentHP != 123 || state.CurrentMP != 45 || state.Location != (location.Location{X: 1, Y: 2, Z: 3}) || state.Heading != 4 {
		t.Fatalf("persisted alive state was overwritten: %+v", state)
	}
}

func TestStateSetStatsSkipsDeadRows(t *testing.T) {
	state := &State{
		Name:        "dead_boss",
		Status:      StatusDead,
		RespawnTime: 9_000,
	}

	state.SetStats(10, 20, location.Location{X: 1, Y: 2, Z: 3}, 4)
	if state.CurrentHP != 0 || state.CurrentMP != 0 || state.Location != (location.Location{}) || state.Heading != 0 || state.RespawnTime != 9_000 {
		t.Fatalf("dead state changed after SetStats(): %+v", state)
	}
}

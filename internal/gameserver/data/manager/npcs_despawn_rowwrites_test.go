package manager

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// countingRowQueue runs each job inline and counts them.
type countingRowQueue struct{ jobs int }

func (q *countingRowQueue) Enqueue(_ int32, job func()) bool {
	q.jobs++
	job()
	return true
}

func (q *countingRowQueue) Flush(context.Context, ...int32) error { return nil }

// recordingRowStore records the row writes and whole saves it receives.
type recordingRowStore struct {
	rowWrites int
	saves     []map[string]*spawn.State
}

func (s *recordingRowStore) LoadStates(context.Context) (map[string]*spawn.State, error) {
	return nil, nil
}

func (s *recordingRowStore) SaveStates(_ context.Context, states map[string]*spawn.State) error {
	s.saves = append(s.saves, states)
	return nil
}

func (s *recordingRowStore) SaveState(context.Context, spawn.State) error {
	s.rowWrites++
	return nil
}

// A despawn of every NPC resets all the database-tracked rows with one
// whole-list save, not a write per row, so a large spawn list neither floods
// the persistence lane nor eats the //respawnall budget (#4685).
func TestDespawnAllWritesTheTrackedRowsInOneSave(t *testing.T) {
	const tracked = 50
	var body strings.Builder
	body.WriteString(`<list>
	<territory name="field" minZ="-10" maxZ="10"><node x="0" y="0"/><node x="100" y="0"/><node x="100" y="100"/><node x="0" y="100"/></territory>`)
	for i := range tracked {
		fmt.Fprintf(&body, `
	<npcmaker name="lair%d" territory="field" maximumNpcs="1">
		<npc id="2" total="1" pos="50;50;0;0" respawn="60sec" dbName="boss_db%d"/>
	</npcmaker>`, i, i)
	}
	body.WriteString("\n</list>")

	spawns := NewSpawns(despawnTable(t, body.String()), nil)
	f := newDespawnFixtureOf(t, spawns)
	queue, store := &countingRowQueue{}, &recordingRowStore{}
	spawns.WriteRows(store, queue, zerolog.Nop())

	f.npcs.DespawnAll()

	if queue.jobs != 1 || store.rowWrites != 0 || len(store.saves) != 1 {
		t.Fatalf("despawn queued %d jobs, %d row writes, %d whole saves; want 1 job, 0 row writes, 1 whole save", queue.jobs, store.rowWrites, len(store.saves))
	}
	saved := store.saves[0]
	if len(saved) != tracked {
		t.Fatalf("whole save carried %d rows, want %d", len(saved), tracked)
	}
	for name, row := range saved {
		if row.Status != spawn.StatusUninitialized {
			t.Fatalf("row %q saved with status %d, want uninitialized", name, row.Status)
		}
	}
}

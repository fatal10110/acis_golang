package scenario

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// makerTemplates are the fixture templates of the npcmaker lines' spawn
// entries that no npc line (known) already builds, one per id.
func (r *run) makerTemplates(t *testing.T, known []*npc.Template) []*npc.Template {
	t.Helper()
	var out []*npc.Template
	for _, m := range r.sc.NpcMakers {
		sp := m.Spawn
		if sp == nil {
			continue
		}
		have := func(tm *npc.Template) bool { return tm.ID == int(sp.NPC) }
		if slices.ContainsFunc(known, have) || slices.ContainsFunc(out, have) {
			continue
		}
		tmpl := template(NPC{ID: sp.NPC, Type: sp.Type})
		if !npc.Attackable(&npc.Instance{Template: tmpl}) {
			t.Fatalf("%s: npcmaker %s spawn: type %s is not an attackable", r.sc.Name, m.Name, sp.Type)
		}
		out = append(out, tmpl)
	}
	return out
}

// makerSpawnOptions starts the makers as the server boots, over the
// spawn_data rows the spawnrow lines left.
func (r *run) makerSpawnOptions(t *testing.T, makers script.MakerCatalog) []gameservertest.Option {
	t.Helper()
	return []gameservertest.Option{
		gameservertest.WithMakers(makers),
		gameservertest.WithSpawnData(),
		gameservertest.WithSpawnDataSeed(func(store *gamesql.SpawnStore) { r.seedSpawnRows(t, store) }),
	}
}

// seedSpawnRows saves the spawnrow lines' rows, as a server that stopped
// left them. The spawns read the wall clock, so a dead row's respawn is that long after now.
func (r *run) seedSpawnRows(t *testing.T, store *gamesql.SpawnStore) {
	t.Helper()
	states := map[string]*spawn.State{}
	for _, row := range r.sc.SpawnRows {
		st := spawn.NewState(row.Name)
		if row.Dead {
			st.SetRespawn(row.Respawn, time.Now())
		} else {
			st.Status, st.CurrentHP, st.CurrentMP = spawn.StatusAlive, 1000, 1000
			st.Location = location.Location{X: row.X, Y: row.Y, Z: row.Z}
		}
		states[row.Name] = st
	}
	if err := store.SaveStates(context.Background(), states); err != nil {
		t.Fatalf("%s: seed spawn data: %v", r.sc.Name, err)
	}
}

// checkStarted compares what the makers sent each other as the server
// started them with the started lines, then drops it: it is no step's.
func (r *run) checkStarted(t *testing.T) {
	t.Helper()
	if !r.sc.StartsMakers() {
		return
	}
	r.srv.Settle(t)
	want := make([]string, len(r.sc.Started))
	for i, e := range r.sc.Started {
		want[i] = makerLine(e.Maker, e.Event, e.Int1, e.Int2)
	}
	if got := r.makerSent.take(); !slices.Equal(got, want) {
		t.Fatalf("%s: the makers started sent %q, want %q (the started lines)", r.sc.Name, strings.Join(got, "; "), strings.Join(want, "; "))
	}
}

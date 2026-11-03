package scenario

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// makerTemplates are the fixture templates of the npcmaker lines' spawn
// entries that no npc line (known) already builds, one per id; each is an
// attackable, which kinds records for the scripts bound to it.
func (r *run) makerTemplates(t *testing.T, known []*npc.Template, kinds map[int32]script.NPCKind) []*npc.Template {
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
		tmpl := template(NPC{ID: sp.NPC, Type: sp.Type, Corpse: sp.Corpse})
		if !npc.Attackable(&npc.Instance{Template: tmpl}) {
			t.Fatalf("%s: npcmaker %s spawn: type %s is not an attackable", r.sc.Name, m.Name, sp.Type)
		}
		kinds[sp.NPC] = script.KindHostile
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

// makerEvent sends the npcmaker args[0] the script event args[1] with the
// integers args[2] and args[3], as a script outside the maker sends it. The
// send itself is the step's line; what the maker's script sends or says on
// is the step's.
func (r *run) makerEvent(t *testing.T, args []string) {
	t.Helper()
	n, _ := ints(args[2:])
	maker := r.sc.maker(args[0]).Name
	r.makerSent.setOwn(makerLine(maker, args[1], int32(n[0]), int32(n[1])))
	found := r.srv.NpcSpawns.MakerEvent(maker, args[1], n[0], n[1])
	r.makerSent.setOwn("")
	if !found {
		t.Fatalf("%s: npcmaker %s is not in the server", r.sc.Name, maker)
	}
	r.srv.Settle(t)
}

// killSpawn lands a lethal hit from the character on the NPC the npcmaker
// name's spawn placed, and lets the kill's time pass.
func (r *run) killSpawn(t *testing.T, name string) {
	t.Helper()
	sp := r.sc.maker(name).Spawn
	var target *npc.Hostile
	for _, obj := range r.srv.State.Objects() {
		h, ok := obj.(*npc.Hostile)
		if !ok || h.Template().ID != int(sp.NPC) || r.rd.placed[h.ObjectID()] || h.AlikeDead() {
			continue
		}
		target = h
	}
	if target == nil {
		t.Fatalf("%s: killspawn %s: no live NPC %d is in the world", r.sc.Name, name, sp.NPC)
	}
	r.onPlayer(t, func(p attackable.Combatant) { target.TakeDamage(target.MaxHP()+1, p) })
	r.passKillTime(t)
}

// passRespawns lets d pass for the respawns the makers' spawns armed on the
// scenario's clock: a respawn that comes due brings its NPC back, as the
// server's respawn task would (the harness never fires one). A respawn is
// seen the first time this runs after it was armed, which is at the end of
// the tick that armed it.
func (r *run) passRespawns(t *testing.T, d time.Duration) {
	t.Helper()
	if !r.sc.StartsMakers() {
		return
	}
	if r.respawnLeft == nil {
		r.respawnLeft = map[task.SlotID]time.Duration{}
	}
	fired := false
	for id := range r.srv.NpcSpawns.Slots() {
		deadline, armed := r.srv.NpcRespawns.Deadline(id)
		if !armed {
			delete(r.respawnLeft, id)
			continue
		}
		left, seen := r.respawnLeft[id]
		if !seen {
			// The armed delay is whole seconds from the wall clock, a few
			// milliseconds ago.
			r.respawnLeft[id] = time.Until(deadline).Round(time.Second)
			continue
		}
		if left -= d; left > 0 {
			r.respawnLeft[id] = left
			continue
		}
		delete(r.respawnLeft, id)
		r.srv.NpcRespawns.Cancel(id)
		r.srv.NpcSpawns.Respawn(id)
		fired = true
	}
	if fired {
		r.srv.Settle(t)
	}
}

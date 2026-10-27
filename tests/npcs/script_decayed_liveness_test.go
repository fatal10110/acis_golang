package npcs

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// decayedLiveness is a behavior bound to the wolf and the grocer whose
// decayed hook records whether its NPC reads dead.
type decayedLiveness struct {
	mu   sync.Mutex
	dead []bool
}

func (d *decayedLiveness) script() script.Script {
	return script.Script{Behavior: true, NPCs: []int32{spawnWolfID, spawnGrocerID}, Hooks: script.Hooks{
		OnDecayed: func(_ *script.Script, e script.Decayed) {
			d.mu.Lock()
			d.dead = append(d.dead, e.NPC.Dead())
			d.mu.Unlock()
		},
	}}
}

// seen returns what the decayed hooks saw so far.
func (d *decayedLiveness) seen() []bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]bool(nil), d.dead...)
}

// A living NPC that leaves at its despawn time or through a script delete
// is already dead when its decayed hook runs, as a decayed corpse is: a
// decayed hook guarded by "not dead" never runs its body (#4256). Monster
// and civilian alike.
func TestScriptDecayedHookSeesALeavingLivingNPCDead(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{spawnWolfID, spawnGrocerID} {
		for _, how := range []string{"despawn", "delete"} {
			t.Run(fmt.Sprintf("%d/%s", id, how), func(t *testing.T) {
				t.Parallel()
				d := &decayedLiveness{}
				kinds := map[int32]script.NPCKind{spawnWolfID: script.KindHostile, spawnGrocerID: script.KindFolk}
				w := bootScriptSpawns(t, gameservertest.WithNPCScripts(kinds,
					[]script.Listing{{Path: "ai.DecayedLiveness"}}, script.Catalog{"ai.DecayedLiveness": d.script}))

				var despawn time.Duration
				if how == "despawn" {
					despawn = 30 * time.Second
				}
				h := w.spawner.AddSpawn(id, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, despawn)
				if h == nil || h.Dead() {
					t.Fatal("the NPC did not spawn alive")
				}
				if how == "despawn" {
					w.srv.Advance(t, despawn)
				} else {
					h.DeleteMe()
				}
				if !h.Decayed() {
					t.Fatal("the NPC did not leave")
				}
				if got := d.seen(); len(got) != 1 || !got[0] {
					t.Fatalf("decayed hooks saw the NPC dead = %v, want [true]", got)
				}
			})
		}
	}
}

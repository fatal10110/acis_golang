package npcs

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// decayTimerName is the timer the decay-timer behavior starts on its NPC.
const decayTimerName = "patrol"

// decayTimers is a behavior bound to one NPC: its created hook starts a
// long timer on the NPC, and its decayed hook records whether that timer
// is still pending.
type decayTimers struct {
	id    int32
	self  atomic.Pointer[script.Script]
	mu    sync.Mutex
	saw   []bool
	fired atomic.Int32
}

func (d *decayTimers) script() script.Script {
	return script.Script{Behavior: true, NPCs: []int32{d.id}, Hooks: script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			d.self.Store(s)
			s.StartTimer(decayTimerName, e.NPC, nil, time.Hour)
		},
		OnDecayed: func(s *script.Script, e script.Decayed) {
			pending := s.HasTimer(decayTimerName, e.NPC, nil)
			d.mu.Lock()
			d.saw = append(d.saw, pending)
			d.mu.Unlock()
		},
		OnTimer: func(*script.Script, script.Timer) string {
			d.fired.Add(1)
			return ""
		},
	}}
}

// seen returns what the decayed hooks saw so far.
func (d *decayTimers) seen() []bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]bool(nil), d.saw...)
}

// A behavior's decayed hook runs before the timers bound to its NPC stop:
// it still sees its timer pending, and the timer is gone once the decay,
// or the script delete, returns. Monster and mortal civilian NPC alike.
func TestScriptDecayedHookSeesItsTimerBeforeItStops(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{lifeWolfID, lifeGrocerID} {
		for _, how := range []string{"corpse", "delete"} {
			t.Run(fmt.Sprintf("%d/%s", id, how), func(t *testing.T) {
				t.Parallel()
				d := &decayTimers{id: id}
				kinds := map[int32]script.NPCKind{lifeWolfID: script.KindHostile, lifeGrocerID: script.KindFolk}
				srv := gameservertest.Boot(t,
					gameservertest.WithNPCs(lifeTemplates()),
					gameservertest.WithNpcSpawns(lifeTable(t)),
					gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "ai.DecayTimers"}}, script.Catalog{"ai.DecayTimers": d.script}),
				)
				target := liveOf(t, srv, int(id)).(attackable.Combatant)
				handle := script.NPCOf(target)
				s := d.self.Load()
				if s == nil || !s.HasTimer(decayTimerName, handle, nil) {
					t.Fatal("the created hook started no timer")
				}

				switch how {
				case "corpse":
					onQueueOf(t, target, func() {
						switch o := target.(type) {
						case *npc.Hostile:
							o.Decay(srv.State, srv.NpcSpawns.RespawnHook(o.ObjectID()))
						case *npc.Folk:
							o.Decay(srv.State, srv.NpcSpawns.RespawnHook(o.ObjectID()))
						}
						if s.HasTimer(decayTimerName, handle, nil) {
							t.Error("the timer is pending once the decay returned")
						}
					})
				case "delete":
					handle.DeleteMe()
					if s.HasTimer(decayTimerName, handle, nil) {
						t.Fatal("the timer is pending once the delete returned")
					}
				}
				if got := d.seen(); len(got) != 1 || !got[0] {
					t.Fatalf("decayed hooks saw the timer pending = %v, want [true]", got)
				}
				srv.Advance(t, time.Hour)
				if n := d.fired.Load(); n != 0 {
					t.Fatalf("the stopped timer fired %d times", n)
				}
			})
		}
	}
}

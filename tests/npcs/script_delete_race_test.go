package npcs

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A mortal civilian NPC's respawn runs its created hooks again, for a new
// NPC on a new object id.
func TestScriptCreatedHooksRunOnFolkRespawn(t *testing.T) {
	t.Parallel()
	w := bootLife(t)
	w.log.take()

	old := w.live(t, lifeGrocerID)
	script.NPCOf(old).DeleteMe()
	if got, want := w.log.take(), []string{"recorder decayed 21203 decayed=true inWorld=true"}; !slices.Equal(got, want) {
		t.Fatalf("delete hooks = %q, want %q", got, want)
	}
	w.srv.NpcSpawns.Respawn(lifeGrocerKey)
	w.srv.Settle(t)
	if got, want := w.log.take(), []string{"recorder created 21203 decayed=false inWorld=true"}; !slices.Equal(got, want) {
		t.Fatalf("respawn hooks = %q, want %q", got, want)
	}
	if now := w.live(t, lifeGrocerID); now.ObjectID() == old.ObjectID() {
		t.Fatal("the respawn reused the deleted NPC's object id")
	}
}

// A script delete of a maker NPC's corpse, from another goroutine, racing
// the corpse decay on the NPC's own queue, decays it once and arms its
// spawn's respawn every time, whichever side wins: monster and mortal
// civilian NPC alike, on the real pool.
func TestScriptDeleteRacingCorpseDecayArmsTheRespawnOnce(t *testing.T) {
	t.Parallel()
	const rounds = 100
	for _, tc := range []struct {
		id  int
		key string
	}{{lifeWolfID, lifeWolfKey}, {lifeGrocerID, lifeGrocerKey}} {
		t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
			t.Parallel()
			w := bootLife(t, gameservertest.WithRealPool())
			killer, _ := w.srv.State.Object(w.player)
			decayed := fmt.Sprintf("recorder decayed %d decayed=true", tc.id)
			for round := range rounds {
				w.srv.Settle(t)
				target := w.live(t, tc.id)
				onQueueOf(t, target, func() {
					switch o := target.(type) {
					case *npc.Hostile:
						o.Kill(killer.(attackable.Combatant))
					case *npc.Folk:
						o.Kill(killer.(attackable.Combatant))
					}
				})
				w.log.take()

				// The corpse decay, as the decay task runs it on the
				// NPC's queue, and the delete start together.
				var wg sync.WaitGroup
				start := make(chan struct{})
				wg.Add(2)
				go func() {
					defer wg.Done()
					<-start
					script.NPCOf(target).DeleteMe()
				}()
				go func() {
					defer wg.Done()
					<-start
					// A delete that already closed the queue refuses the
					// decay, as it refuses the decay task's.
					done := make(chan struct{})
					if !queueOf(target).Post(func() {
						defer close(done)
						switch o := target.(type) {
						case *npc.Hostile:
							o.DecayWithRespawn(w.srv.State, w.srv.NpcSpawns.RespawnHook)
						case *npc.Folk:
							o.DecayWithRespawn(w.srv.State, w.srv.NpcSpawns.RespawnHook)
						}
					}) {
						return
					}
					<-done
				}()
				close(start)
				wg.Wait()
				w.srv.Settle(t)

				n := 0
				for _, line := range w.log.take() {
					if strings.HasPrefix(line, decayed) {
						n++
					}
				}
				if n != 1 {
					t.Fatalf("round %d: decayed hooks ran %d times, want 1", round, n)
				}
				if !w.srv.NpcRespawns.Cancel(tc.key) {
					t.Fatalf("round %d: the racing delete and decay armed no respawn", round)
				}
				w.srv.NpcSpawns.Respawn(tc.key)
			}
		})
	}
}

// queueOf returns the NPC's own queue.
func queueOf(n attackable.Combatant) interface{ Post(func()) bool } {
	switch o := n.(type) {
	case *npc.Hostile:
		return o.Queue()
	case *npc.Folk:
		return o.Queue()
	}
	return nil
}

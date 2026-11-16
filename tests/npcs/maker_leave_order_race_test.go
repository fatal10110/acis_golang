package npcs

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPCs of one npcmaker leaving at once, on their own queues on the real
// pool, reach its NPC-deleted hook one after the other, as sequential
// decays do: each hook sees the spawn as the earlier leaves' bookkeeping
// left it and the later leavers still in the world (#5204).

const (
	leaveOrderID   = 21997
	leaveOrderList = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="order_hall" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="leave_order" territory="order_hall" maximumNpcs="2">
		<ai type="leave_order_probe"/>
		<npc id="21997" total="2" pos="40;20;30;0" respawn="0sec"/>
	</npcmaker>
</list>`
)

// spawnView is what an NPC-deleted hook sees of its group and spawn.
type spawnView struct{ alive, spawned, decayed, listed int }

func viewOf(e script.MakerNPC) spawnView {
	return spawnView{alive: e.Group.Alive(), spawned: e.Spawn.Spawned(), decayed: e.Spawn.Decayed(), listed: len(e.Spawn.NPCs())}
}

// leaveOrderProbe records the view of every NPC-deleted hook; deleteRest
// has a hook that finds others alive delete them through its spawn's NPC
// handles, recording the view again once they are gone, where without it
// the hook that finds none left spawns the pair again.
type leaveOrderProbe struct {
	deleteRest bool
	mu         sync.Mutex
	views      []string
}

func (p *leaveOrderProbe) record(format string, args ...any) {
	p.mu.Lock()
	p.views = append(p.views, fmt.Sprintf(format, args...))
	p.mu.Unlock()
}

func (p *leaveOrderProbe) take() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.views
	p.views = nil
	return out
}

func (p *leaveOrderProbe) makers() script.MakerCatalog {
	return script.MakerCatalog{"leave_order_probe": func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnStart: func(_ *script.Maker, e script.MakerStart) {
				e.Group.Spawns()[0].Spawn()
				e.Group.Spawns()[0].Spawn()
			},
			OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
				v := viewOf(e)
				p.record("deleted %+v", v)
				switch {
				case p.deleteRest && v.alive > 0:
					for _, other := range e.Spawn.NPCs() {
						if !other.Decayed() {
							other.Delete()
						}
					}
					p.record("after delete %+v", viewOf(e))
				case !p.deleteRest && v.alive == 0:
					e.Spawn.Spawn()
					e.Spawn.Spawn()
				}
			},
		}}
	}}
}

func bootLeaveOrder(t *testing.T, probe *leaveOrderProbe) *makerWorld {
	t.Helper()
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithRealPool(),
		gameservertest.WithNPCs(batch1Templates(leaveOrderID)),
		gameservertest.WithNpcSpawns(batch1Table(t, leaveOrderList)),
		gameservertest.WithMakers(probe.makers()),
	)
	return &makerWorld{srv: w.srv}
}

// Both NPCs of a spawn that never respawns leave at once: the first hook
// sees its own slot listed decayed and the other NPC alive, the last only
// its own slot, the first one's dropped, as when they leave one after the
// other. The last hook spawns the pair again, so a slot left listed would
// show in the next round.
func TestMakerNPCsLeavingAtOnceRunTheirHooksInLeaveOrder(t *testing.T) {
	t.Parallel()
	const rounds = 50
	probe := &leaveOrderProbe{}
	mw := bootLeaveOrder(t, probe)
	want := []string{
		fmt.Sprintf("deleted %+v", spawnView{alive: 1, spawned: 1, decayed: 1, listed: 2}),
		fmt.Sprintf("deleted %+v", spawnView{alive: 0, spawned: 0, decayed: 1, listed: 1}),
	}
	for round := range rounds {
		mw.srv.Settle(t)
		npcs := mw.inWorld(leaveOrderID)
		if len(npcs) != 2 {
			t.Fatalf("round %d: NPCs = %d, want 2", round, len(npcs))
		}
		probe.take()
		for _, h := range npcs {
			h.DeleteMe()
		}
		mw.srv.Settle(t)
		if got := probe.take(); !slices.Equal(got, want) {
			t.Fatalf("round %d, both left at once: hooks saw %q, want %q", round, got, want)
		}
	}
}

// A hook that deletes another NPC of its own npcmaker through its spawn's
// handle runs that NPC's hook before it carries on, its own slot still
// listed, and finds it gone, its slot dropped: no deadlock on the group's
// leave order.
func TestMakerHookDeletingItsOwnGroupsNPCRunsThatHookFirst(t *testing.T) {
	t.Parallel()
	probe := &leaveOrderProbe{deleteRest: true}
	mw := bootLeaveOrder(t, probe)
	mw.srv.Settle(t)
	npcs := mw.inWorld(leaveOrderID)
	if len(npcs) != 2 {
		t.Fatalf("NPCs = %d, want 2", len(npcs))
	}
	npcs[0].DeleteMe()
	mw.srv.Settle(t)
	want := []string{
		fmt.Sprintf("deleted %+v", spawnView{alive: 1, spawned: 1, decayed: 1, listed: 2}),
		fmt.Sprintf("deleted %+v", spawnView{alive: 0, spawned: 0, decayed: 2, listed: 2}),
		fmt.Sprintf("after delete %+v", spawnView{alive: 0, spawned: 0, decayed: 1, listed: 1}),
	}
	if got := probe.take(); !slices.Equal(got, want) {
		t.Fatalf("hook deleting the other NPC: hooks saw %q, want %q", got, want)
	}
	if got := len(mw.inWorld(leaveOrderID)); got != 0 {
		t.Fatalf("NPCs after the hook deleted the other = %d, want 0", got)
	}
}

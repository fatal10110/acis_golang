package npcs

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Two NPCs of one Royal Rush room leaving at once, each on its own
// goroutine, clear the room once: only the last to leave finds none left,
// so the next room hears 1001 once (#5094).

const (
	lastNPCGuardID = 21994
	lastNPCList    = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="royal_hall" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="royal_clock" territory="royal_hall" maximumNpcs="1">
		<ai type="royal_clock_probe"/>
	</npcmaker>
	<npcmaker name="last_npc_room" territory="royal_hall" maximumNpcs="2">
		<ai type="royal_req_next_maker">
			<set name="req_count" val="0"/>
			<set name="next_maker_name" val="last_npc_next"/>
		</ai>
		<npc id="21994" total="2" pos="40;20;30;0" respawn="0sec"/>
	</npcmaker>
	<npcmaker name="last_npc_next" territory="royal_hall" maximumNpcs="1">
		<ai type="last_npc_next_probe"/>
	</npcmaker>
</list>`
)

// lastNPCProbe records the script events the next room hears, and has the
// first guard to leave make the other one leave on another goroutine while
// its own decay runs, before the room takes its leave.
type lastNPCProbe struct {
	srv   atomic.Pointer[gameservertest.Server]
	first atomic.Bool
	mu    sync.Mutex
	heard []string
}

func (p *lastNPCProbe) take() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.heard
	p.heard = nil
	return out
}

func (p *lastNPCProbe) makers(clock *royalClock) script.MakerCatalog {
	out := clock.catalog()
	out["last_npc_next_probe"] = func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
			p.mu.Lock()
			p.heard = append(p.heard, e.Name)
			p.mu.Unlock()
		}}}
	}
	return out
}

func (p *lastNPCProbe) guard() script.Script {
	return script.Script{Behavior: true, NPCs: []int32{lastNPCGuardID}, Hooks: script.Hooks{
		OnDecayed: func(_ *script.Script, e script.Decayed) {
			if !p.first.CompareAndSwap(false, true) {
				return
			}
			mw := &makerWorld{srv: p.srv.Load()}
			done := make(chan struct{})
			go func() {
				defer close(done)
				for _, other := range mw.inWorld(lastNPCGuardID) {
					if other.ObjectID() != e.NPC.ObjectID() {
						other.DeleteNow()
					}
				}
			}()
			<-done
		},
	}}
}

func TestRoyalRushRoomClearedByTwoGuardsLeavingAtOnceCallsTheNextRoomOnce(t *testing.T) {
	t.Parallel()
	clock := &royalClock{}
	probe := &lastNPCProbe{}
	kinds := map[int32]script.NPCKind{lastNPCGuardID: script.KindHostile}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(lastNPCGuardID)),
		gameservertest.WithNpcSpawns(batch1Table(t, lastNPCList)),
		gameservertest.WithMakers(probe.makers(clock)),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "ai.LastNPCGuard"}}, script.Catalog{"ai.LastNPCGuard": probe.guard}),
	)
	if !w.srv.DrivesClock() {
		t.Skip("the wall clock cannot be moved to a given minute on the real pool")
	}
	probe.srv.Store(w.srv)
	mw := &makerWorld{srv: w.srv}

	clock.toTick(t, w.srv, 10, 0)
	if !w.srv.NpcSpawns.MakerEvent("last_npc_room", "1001", 0, 0) {
		t.Fatal("npcmaker last_npc_room not found by name")
	}
	w.srv.Advance(t, time.Millisecond)
	guards := mw.inWorld(lastNPCGuardID)
	if len(guards) != 2 {
		t.Fatalf("1001 at minute 10: guards = %d, want 2", len(guards))
	}
	probe.take()

	guards[0].DeleteMe()
	w.srv.Settle(t)
	w.srv.Advance(t, time.Millisecond)
	if got := len(mw.inWorld(lastNPCGuardID)); got != 0 {
		t.Fatalf("both guards left: guards = %d, want 0", got)
	}
	if got, want := probe.take(), []string{"1001"}; !slices.Equal(got, want) {
		t.Fatalf("both guards left at once: the next room heard %q, want %q", got, want)
	}
}

const (
	lastNPCPairID   = 21998
	lastNPCPairList = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="pair_hall" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="last_npc_pair" territory="pair_hall" maximumNpcs="2">
		<ai type="last_npc_pair_probe"/>
		<npc id="21998" total="2" pos="40;20;30;0" respawn="0sec"/>
	</npcmaker>
</list>`
)

// The two NPCs of one npcmaker leaving at once on their own queues, on the
// real pool, run its "none left" branch once each time. The fixture maker
// type spawns the pair at start and again from the hook that finds none
// left, counting those hooks.
func TestMakerPairLeavingAtOnceFindsNoneLeftOnce(t *testing.T) {
	t.Parallel()
	const rounds = 50
	var lasts atomic.Int32
	makers := script.MakerCatalog{"last_npc_pair_probe": func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnStart: func(_ *script.Maker, e script.MakerStart) {
				e.Group.Spawns()[0].Spawn()
				e.Group.Spawns()[0].Spawn()
			},
			OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
				if e.Group.Alive() == 0 {
					lasts.Add(1)
					e.Spawn.Spawn()
					e.Spawn.Spawn()
				}
			},
		}}
	}}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithRealPool(),
		gameservertest.WithNPCs(batch1Templates(lastNPCPairID)),
		gameservertest.WithNpcSpawns(batch1Table(t, lastNPCPairList)),
		gameservertest.WithMakers(makers),
	)
	mw := &makerWorld{srv: w.srv}
	for round := range rounds {
		w.srv.Settle(t)
		npcs := mw.inWorld(lastNPCPairID)
		if len(npcs) != 2 {
			t.Fatalf("round %d: NPCs = %d, want 2", round, len(npcs))
		}
		lasts.Store(0)
		for _, h := range npcs {
			h.DeleteMe()
		}
		w.srv.Settle(t)
		if got := lasts.Load(); got != 1 {
			t.Fatalf("round %d, both left at once: hooks finding none left = %d, want 1", round, got)
		}
	}
}

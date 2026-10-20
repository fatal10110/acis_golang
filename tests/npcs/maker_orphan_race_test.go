package npcs

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A DeleteAll that another caller makes while an NPC's deleted hook runs
// lets the NPC go: the respawn the hook then arms for it brings nothing
// back (#4038).

const (
	orphanRaceBossID  = 21991
	orphanRaceSpawnls = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="orphan_race_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="orphan_race" territory="orphan_race_field" maximumNpcs="1">
		<ai type="orphan_race_probe"/>
		<npc id="21991" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`
)

// orphanRaceProbe is a fixture maker type: it starts its spawn, and
// answers its NPC leaving by having another goroutine run script event
// "delete" (a DeleteAll) on the maker, waiting for it, then arming a 10
// second respawn of the NPC. Each hook logs the group's alive count.
type orphanRaceProbe struct {
	external func()
	mu       sync.Mutex
	log      []string
}

func (p *orphanRaceProbe) add(format string, args ...any) {
	p.mu.Lock()
	p.log = append(p.log, fmt.Sprintf(format, args...))
	p.mu.Unlock()
}

func (p *orphanRaceProbe) take() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	got := p.log
	p.log = nil
	return got
}

func (p *orphanRaceProbe) catalog() script.MakerCatalog {
	return script.MakerCatalog{"orphan_race_probe": func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnStart: func(_ *script.Maker, e script.MakerStart) {
				e.Group.Spawns()[0].Spawn()
			},
			OnNPCCreated: func(_ *script.Maker, e script.MakerNPC) {
				p.add("created alive=%d", e.Group.Alive())
			},
			OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
				done := make(chan struct{})
				go func() {
					defer close(done)
					p.external()
				}()
				<-done
				e.NPC.ScheduleRespawn(10 * time.Second)
				p.add("deleted alive=%d", e.Group.Alive())
			},
			OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
				if e.Name == "delete" {
					e.Group.DeleteAll()
				}
				p.add("%s alive=%d", e.Name, e.Group.Alive())
			},
		}}
	}}
}

func TestMakerRespawnArmedAfterAnotherCallersDeleteAllBringsNothingBack(t *testing.T) {
	t.Parallel()
	probe := &orphanRaceProbe{}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(batch1Templates(orphanRaceBossID)),
		gameservertest.WithNpcSpawns(batch1Table(t, orphanRaceSpawnls)),
		gameservertest.WithMakers(probe.catalog()),
	)
	probe.external = func() {
		if !w.srv.NpcSpawns.MakerEvent("orphan_race", "delete", 0, 0) {
			t.Error("npcmaker orphan_race not found by name")
		}
	}
	mw := &makerWorld{srv: w.srv}
	if got, want := probe.take(), []string{"created alive=1"}; !slices.Equal(got, want) {
		t.Fatalf("boot: maker hooks = %q, want %q", got, want)
	}

	mw.inWorld(orphanRaceBossID)[0].DeleteMe()
	w.srv.Settle(t)
	if got, want := probe.take(), []string{"delete alive=0", "deleted alive=0"}; !slices.Equal(got, want) {
		t.Fatalf("the boss left: maker hooks = %q, want %q", got, want)
	}
	const slot = "orphan_race#0#0"
	if w.srv.NpcRespawns.Tracked(slot) {
		t.Fatal("the boss left: its respawn is armed though another caller's DeleteAll let it go")
	}
	w.srv.NpcSpawns.Respawn(slot)
	w.srv.Settle(t)
	if got := len(mw.inWorld(orphanRaceBossID)); got != 0 {
		t.Fatalf("its slot gone: bosses in the world = %d, want 0", got)
	}
	if got := probe.take(); len(got) != 0 {
		t.Fatalf("its slot gone: maker hooks = %q, want none", got)
	}
}

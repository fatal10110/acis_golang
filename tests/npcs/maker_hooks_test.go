package npcs

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// What a maker hook reads besides its group (#3961, #3962, #3975): the
// template alias of a spawn's NPCs, a random draw from the scripts' random
// source, and the local wall-clock time from the clock the population runs
// on, which a behavior hook reads too.
const (
	hooksManagerID = 21951 // the spawn's NPC whose template has an alias
	hooksPlainID   = 21952 // a template with no alias
)

const hooksSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="hooks_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="hooks_probe" territory="hooks_field" maximumNpcs="2">
		<ai type="hooks_probe_maker"/>
		<npc id="21951" total="1" pos="40;20;30;0" respawn="60sec"/>
		<npc id="21952" total="1" pos="80;20;30;0" respawn="60sec"/>
		<npc id="21953" total="1" pos="120;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// hooksProbe records what the probe maker and the probe behavior read.
type hooksProbe struct {
	mu sync.Mutex
	// group is the probe npcmaker as its start hook was handed it.
	group spawn.Group
	// spawnAliases are the aliases the start hook read off every spawn, in
	// declaration order; createdAliases the alias the created hook read for
	// each NPC entering the world, in creation order.
	spawnAliases, createdAliases []string
	// drawn is what the start hook drew with Rnd(3).
	drawn int
	// started is the time the start hook read; ticks the time each of its
	// one-second timer's runs read.
	started time.Time
	ticks   []time.Time
	// heard is the time the behavior read when the maker's timer, at
	// minute 54 second 0, sent its NPC a script event.
	heard []time.Time
}

func (p *hooksProbe) makers() script.MakerCatalog {
	return script.MakerCatalog{"hooks_probe_maker": func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnStart: func(m *script.Maker, e script.MakerStart) {
				p.mu.Lock()
				p.group = e.Group
				p.started = m.Now()
				p.drawn = m.Rnd(3)
				for _, ms := range e.Group.Spawns() {
					p.spawnAliases = append(p.spawnAliases, ms.Alias())
				}
				p.mu.Unlock()
				for _, ms := range e.Group.Spawns() {
					ms.Spawn()
				}
				e.Group.Every(time.Second, func() {
					now := m.Now()
					p.mu.Lock()
					p.ticks = append(p.ticks, now)
					p.mu.Unlock()
					if now.Minute() == 54 && now.Second() == 0 {
						e.Group.Spawns()[0].SendScriptEvent(54, 0, 0)
					}
				})
			},
			OnNPCCreated: func(_ *script.Maker, e script.MakerNPC) {
				p.mu.Lock()
				p.createdAliases = append(p.createdAliases, e.Spawn.Alias())
				p.mu.Unlock()
			},
		}}
	}}
}

func (p *hooksProbe) behavior() script.Script {
	return script.Script{
		Bind: script.Bindings{script.EventScriptEvent: {hooksManagerID}},
		Hooks: script.Hooks{OnScriptEvent: func(s *script.Script, _ script.ScriptEvent) {
			now := s.Now()
			p.mu.Lock()
			p.heard = append(p.heard, now)
			p.mu.Unlock()
		}},
	}
}

func bootHooksProbe(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, *hooksProbe) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hooks.xml"), []byte(hooksSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	manager := spawnMonster(hooksManagerID, "Manager")
	manager.Alias = "Hooks_Manager"
	p := &hooksProbe{}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{manager, spawnMonster(hooksPlainID, "Plain")})),
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithMakers(p.makers()),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{hooksManagerID: script.KindHostile},
			[]script.Listing{{Path: "ai.HooksProbe"}}, script.Catalog{"ai.HooksProbe": p.behavior}),
	}, opts...)...)
	srv.Settle(t)
	return srv, p
}

// A maker reads the alias of a spawn's template as the template has it,
// case kept, and the empty string for a template with no alias or a spawn
// whose template is unknown (which places nothing); the created hook reads
// the alias of the spawn whose NPC entered the world.
func TestMakerHookReadsSpawnAlias(t *testing.T) {
	t.Parallel()
	_, p := bootHooksProbe(t)
	p.mu.Lock()
	defer p.mu.Unlock()
	if want := []string{"Hooks_Manager", "", ""}; !slices.Equal(p.spawnAliases, want) {
		t.Fatalf("spawn aliases = %q, want %q", p.spawnAliases, want)
	}
	if want := []string{"Hooks_Manager", ""}; !slices.Equal(p.createdAliases, want) {
		t.Fatalf("created hook aliases = %q, want %q", p.createdAliases, want)
	}
}

// A maker's random draw is made through the scripts' random source, which
// a test pins: Rnd(3) asks it for a draw in [0, 3) and returns its answer.
func TestMakerHookDrawsFromScriptRandom(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var asked []int
	_, p := bootHooksProbe(t, gameservertest.WithScriptRand(func(n int) int {
		mu.Lock()
		asked = append(asked, n)
		mu.Unlock()
		return n - 1
	}))
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(asked, []int{3}) {
		t.Fatalf("draws asked of the script random source = %v, want one Rnd(3)", asked)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.drawn != 2 {
		t.Fatalf("maker drew %d, want the pinned 2", p.drawn)
	}
}

// A maker reads the local wall-clock time from the clock its timers run on:
// each run of a one-second timer reads its own deadline, so a test that
// moves the clock reaches minute 54 second 0 with no wall-clock wait. A
// behavior hook the timer raises at that instant reads the same time.
func TestMakerHookReadsWallClock(t *testing.T) {
	t.Parallel()
	srv, p := bootHooksProbe(t)
	if !srv.DrivesClock() {
		t.Skip("the wall clock cannot be moved to minute 54 on the real pool")
	}
	p.mu.Lock()
	started := p.started
	p.mu.Unlock()
	if started.IsZero() || started.Location() != time.Local {
		t.Fatalf("start hook read %v, want a local time", started)
	}
	// target is the first minute 54 second 0 of the local clock after the
	// first timer run; run n of the timer, due n seconds after the start
	// hook, is the first to read it.
	target := started.Add(time.Second).Truncate(time.Hour).Add(54 * time.Minute)
	for target.Before(started.Add(time.Second)) {
		target = target.Add(time.Hour)
	}
	n := int((target.Sub(started) + time.Second - 1) / time.Second)
	srv.Advance(t, time.Duration(n)*time.Second)

	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.ticks) < n {
		t.Fatalf("timer ran %d times, want at least %d", len(p.ticks), n)
	}
	for i, at := range p.ticks {
		if want := started.Add(time.Duration(i+1) * time.Second); !at.Equal(want) {
			t.Fatalf("timer run %d read %v, want its deadline %v", i+1, at, want)
		}
	}
	hit := p.ticks[n-1]
	if hit.Minute() != 54 || hit.Second() != 0 {
		t.Fatalf("timer run %d read %v, want minute 54 second 0", n, hit)
	}
	if len(p.heard) != 1 || !p.heard[0].Equal(hit) {
		t.Fatalf("behavior read %v, want once the timer's %v", p.heard, hit)
	}
}

package npcs

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The npcmakers keep the spawnlist's declaration order: they start in it,
// and a lookup taking the first match finds them in it (#4039).

const (
	listOrderLateID  = 21996
	listOrderEarlyID = 21997
	listOrderShared  = 21998
	listOrderAlias   = "Order_Shared"
	listOrderSpawns  = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="order_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="zz_first" territory="order_field" maximumNpcs="2">
		<ai type="order_probe"/>
		<npc id="21996" total="1" pos="40;20;30;0" respawn="60sec"/>
		<npc id="21998" total="1" pos="100;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="mm_second" territory="order_field" maximumNpcs="2">
		<ai type="order_probe"/>
		<npc id="21997" total="1" pos="60;20;30;0" respawn="60sec"/>
		<npc id="21998" total="1" pos="200;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="aa_third" territory="order_field" maximumNpcs="1">
		<ai type="order_probe"/>
	</npcmaker>
</list>`
)

// orderProbe is a fixture maker type logging each npcmaker's start, and on
// script event "find" which npcmaker's spawn the first-match alias lookup
// returns, by the NPC it places.
type orderProbe struct {
	mu  sync.Mutex
	log []string
}

func (p *orderProbe) add(line string) {
	p.mu.Lock()
	p.log = append(p.log, line)
	p.mu.Unlock()
}

func (p *orderProbe) take() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	got := p.log
	p.log = nil
	return got
}

func (p *orderProbe) catalog() script.MakerCatalog {
	return script.MakerCatalog{"order_probe": func() script.Maker {
		return script.Maker{MakerHooks: script.MakerHooks{
			OnStart: func(_ *script.Maker, e script.MakerStart) {
				p.add("start " + e.Group.Maker().Name)
				for _, s := range e.Group.Spawns() {
					s.Spawn()
				}
			},
			OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
				found := e.Group.FindSpawn(listOrderAlias)
				if found == nil {
					p.add("find none")
					return
				}
				at, _ := found.DrawSpawnLocation()
				p.add(fmt.Sprintf("find %d at x=%d", found.Entry().NPCID, at.Location.X))
			},
		}}
	}}
}

// Three npcmakers declared out of name order start in declaration order;
// the first-match lookups, by alias from a maker and by NPC id from a
// script, find the spawn of the first one declared that has the NPC.
func TestMakersKeepDeclarationOrder(t *testing.T) {
	t.Parallel()
	probe := &orderProbe{}
	alias := map[int]string{listOrderShared: listOrderAlias}
	w := bootFolkWorld(t, map[string]string{},
		gameservertest.WithNPCs(aliasTemplates(alias, listOrderLateID, listOrderEarlyID, listOrderShared)),
		gameservertest.WithNpcSpawns(batch1Table(t, listOrderSpawns)),
		gameservertest.WithMakers(probe.catalog()),
	)
	if got, want := probe.take(), []string{"start zz_first", "start mm_second", "start aa_third"}; !slices.Equal(got, want) {
		t.Fatalf("boot: npcmakers started = %q, want %q", got, want)
	}
	var makers []string
	for _, m := range w.srv.NpcSpawns.Spawns().Table().Makers() {
		makers = append(makers, m.Name)
	}
	if want := []string{"zz_first", "mm_second", "aa_third"}; !slices.Equal(makers, want) {
		t.Fatalf("spawn table makers = %q, want %q", makers, want)
	}

	if !w.srv.NpcSpawns.MakerEvent("aa_third", "find", 0, 0) {
		t.Fatal("npcmaker aa_third not found by name")
	}
	w.srv.Settle(t)
	if got, want := probe.take(), []string{"find 21998 at x=100"}; !slices.Equal(got, want) {
		t.Fatalf("alias lookup = %q, want %q", got, want)
	}

	found := w.srv.NpcSpawns.FindSpawn(listOrderShared)
	if found == nil {
		t.Fatalf("FindSpawn(%d) = nil", listOrderShared)
	}
	at, ok := found.DrawSpawnLocation()
	if !ok || at.Location.X != 100 {
		t.Fatalf("FindSpawn(%d) places at %+v (%v), want x=100 (zz_first's spawn)", listOrderShared, at, ok)
	}
}

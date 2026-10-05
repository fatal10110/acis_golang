package manager

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// names returns the template names of the live Hostiles of npcID.
func (f *despawnFixture) names(npcID int) []string {
	var out []string
	for _, obj := range f.npcObjects() {
		if h, ok := obj.(*npc.Hostile); ok && h.Instance.Template.ID == npcID {
			out = append(out, h.Instance.Template.Name)
		}
	}
	return out
}

// A spawn keeps the template it was built with for life (ASpawn._template
// is final): after //reload npc swaps the table, a maker spawn's and a
// database-tracked spawn's respawns are still the old template. Only spawns
// built afterwards, as //respawnall builds them, use the reloaded one.
func TestRespawnKeepsTheSpawnTemplateAcrossReload(t *testing.T) {
	f := newDespawnFixture(t, nil)
	f.templates.Replace(npc.NewTable([]*npc.Template{
		{ID: 1, TemplateID: 1, Type: "Monster", Name: "Reloaded Wolf", HPMax: 100, RunSpeed: 100},
		{ID: 2, TemplateID: 2, Type: "Monster", Name: "Reloaded Boss", HPMax: 100, RunSpeed: 100},
	}))

	f.hostile(t, 1).DeleteMe()
	f.hostile(t, 2).DeleteMe()
	f.queues.Run()
	wolfKey := "wolves#0#0"
	if !f.respawn.Tracked(wolfKey) {
		wolfKey = "wolves#0#1"
	}
	if !f.respawn.Tracked(wolfKey) || !f.respawn.Tracked("boss_db") {
		t.Fatal("removed wolf and boss armed no respawn")
	}
	f.npcs.Respawn(wolfKey)
	f.npcs.Respawn("boss_db")
	f.queues.Run()

	if got := f.names(1); len(got) != 2 || got[0] != "Wolf" || got[1] != "Wolf" {
		t.Fatalf("wolves after respawn = %v, want two Wolf", got)
	}
	if got := f.names(2); len(got) != 1 || got[0] != "Boss" {
		t.Fatalf("boss after respawn = %v, want Boss", got)
	}

	f.npcs.DespawnAll()
	f.queues.Run()
	f.npcs.RespawnAll(NewSpawns(despawnTable(t, despawnSpawnlist), nil))
	f.queues.Run()
	if got := f.names(1); len(got) != 2 || got[0] != "Reloaded Wolf" || got[1] != "Reloaded Wolf" {
		t.Fatalf("wolves after RespawnAll = %v, want two Reloaded Wolf", got)
	}
	if got := f.names(2); len(got) != 1 || got[0] != "Reloaded Boss" {
		t.Fatalf("boss after RespawnAll = %v, want Reloaded Boss", got)
	}
}

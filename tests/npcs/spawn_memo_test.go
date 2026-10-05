package npcs

import (
	"os"
	"path/filepath"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The memo den's NPCs and their spawn slot keys.
const (
	memoWolfID    = 20120
	memoGrocerID  = 30001
	memoWolfKey   = "memo_den#0#0"
	memoGrocerKey = "memo_den#1#0"
)

// memoTemplates are a monster and a merchant whose templates set AI
// parameters the den's entries partly shadow.
func memoTemplates() *npc.Table {
	wolf := &npc.Template{
		ID: memoWolfID, TemplateID: memoWolfID, Type: "Monster", Name: "Wolf", Level: 1, HPMax: 100,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true,
		AIParams: npc.AIParams{"Shared": "1", "TemplateOnly": "2", "SuperPointName": "template_route"},
	}
	grocer := gameservertest.FolkTemplate("Merchant", memoGrocerID)
	grocer.AIParams = npc.AIParams{"Shared": "template", "TemplateOnly": "t"}
	return npc.NewTable([]*npc.Template{wolf, grocer})
}

// memoDen is a maker of one wolf and one grocer, each entry with its own
// <ai> values.
func memoDen(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	body := `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="den" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="memo_den" territory="den" maximumNpcs="2">
		<npc id="20120" total="1" pos="200;300;30;0" respawn="60sec"><ai><set name="Shared" val="10"/><set name="SuperPointName" val="den_route"/></ai></npc>
		<npc id="30001" total="1" pos="220;300;30;0" respawn="60sec"><ai><set name="Shared" val="spawn"/></ai></npc>
	</npcmaker>
</list>`
	if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

// liveOf returns the one live NPC of template id in the world.
func liveOf(t *testing.T, srv *gameservertest.Server, id int) world.Tracked {
	t.Helper()
	var found []world.Tracked
	for _, obj := range srv.State.Objects() {
		var inst *npc.Instance
		switch o := obj.(type) {
		case *npc.Hostile:
			inst = o.Instance
		case *npc.Folk:
			inst = o.Instance
		}
		if inst != nil && inst.Template.ID == id {
			found = append(found, obj)
		}
	}
	if len(found) != 1 {
		t.Fatalf("live NPCs of template %d = %d, want 1", id, len(found))
	}
	return found[0]
}

// A spawned NPC reads its AI parameters from its spawn entry first, then
// from its template, then the caller's default; SuperPointName comes from
// the spawn entry.
func TestSpawnMemoShadowsTemplateAIParams(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithNPCs(memoTemplates()), gameservertest.WithNpcSpawns(memoDen(t)))

	wolf := liveOf(t, w.srv, memoWolfID).(*npc.Hostile)
	if got := wolf.AIInt("Shared", 0); got != 10 {
		t.Errorf("wolf Shared = %d, want the spawn's 10", got)
	}
	if got := wolf.AIInt("TemplateOnly", 0); got != 2 {
		t.Errorf("wolf TemplateOnly = %d, want the template's 2", got)
	}
	if got := wolf.AIInt("Missing", 4); got != 4 {
		t.Errorf("wolf Missing = %d, want the default 4", got)
	}
	if got := wolf.AIString("SuperPointName", ""); got != "den_route" {
		t.Errorf("wolf SuperPointName = %q, want the spawn's den_route", got)
	}

	grocer := liveOf(t, w.srv, memoGrocerID).(*npc.Folk)
	if got := grocer.AIString("Shared", ""); got != "spawn" {
		t.Errorf("grocer Shared = %q, want the spawn's", got)
	}
	if got := grocer.AIString("TemplateOnly", ""); got != "t" {
		t.Errorf("grocer TemplateOnly = %q, want the template's", got)
	}
	if got := grocer.AIString("Missing", "d"); got != "d" {
		t.Errorf("grocer Missing = %q, want the default", got)
	}
}

// fillScratch writes a recognizable value into some slots of s.
func fillScratch(s *npc.Scratch, creature npc.ScratchCreature) {
	s.SetInt(npc.IntAI0, 11)
	s.SetInt(npc.IntQuest4, 44)
	s.SetInt(npc.IntWeightPoint, 5)
	s.SetCreature(npc.CreatureAI1, creature)
	s.SetScriptValue(3)
}

// checkCarried fails unless s holds what fillScratch wrote, with the
// script value cleared.
func checkCarried(t *testing.T, who string, s *npc.Scratch, creature npc.ScratchCreature) {
	t.Helper()
	if got := s.ScriptValue(); got != 0 {
		t.Errorf("%s script value after respawn = %d, want 0", who, got)
	}
	if a0, q4, wp := s.Int(npc.IntAI0), s.Int(npc.IntQuest4), s.Int(npc.IntWeightPoint); a0 != 11 || q4 != 44 || wp != 5 {
		t.Errorf("%s int slots after respawn = %d, %d, %d; want 11, 44, 5", who, a0, q4, wp)
	}
	if got := s.Creature(npc.CreatureAI1); got != creature {
		t.Errorf("%s creature slot after respawn = %v, want %v", who, got, creature)
	}
}

// The script memory belongs to the spawn slot: the NPC a slot respawns
// finds what the previous one left, except the script value, which a new
// life clears. Both a monster and a civilian carry it.
func TestSpawnSlotScratchSurvivesRespawn(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithNPCs(memoTemplates()), gameservertest.WithNpcSpawns(memoDen(t)))
	srv := w.srv

	wolf := liveOf(t, srv, memoWolfID).(*npc.Hostile)
	grocer := liveOf(t, srv, memoGrocerID).(*npc.Folk)
	if wolf.Scratch() == grocer.Scratch() {
		t.Fatal("two spawn slots share one script memory")
	}
	if got := wolf.Scratch().Int(npc.IntWeightPoint); got != 1 {
		t.Fatalf("fresh weight point = %d, want 1", got)
	}
	fillScratch(wolf.Scratch(), grocer)
	fillScratch(grocer.Scratch(), wolf)

	wolf.DeleteMe()
	grocerID := grocer.ObjectID()
	grocer.Queue().Post(func() { grocer.Decay(srv.State, srv.NpcSpawns.RespawnHook(grocerID)) })
	srv.Settle(t)
	for _, key := range []string{memoWolfKey, memoGrocerKey} {
		if !srv.NpcRespawns.Tracked(key) {
			t.Fatalf("slot %s armed no respawn", key)
		}
		srv.NpcSpawns.Respawn(key)
	}
	srv.Settle(t)

	newWolf := liveOf(t, srv, memoWolfID).(*npc.Hostile)
	newGrocer := liveOf(t, srv, memoGrocerID).(*npc.Folk)
	if newWolf.ObjectID() == wolf.ObjectID() || newGrocer.ObjectID() == grocerID {
		t.Fatal("respawn reused the object id")
	}
	checkCarried(t, "wolf", newWolf.Scratch(), grocer)
	checkCarried(t, "grocer", newGrocer.Scratch(), wolf)
	if got := newWolf.AIString("SuperPointName", ""); got != "den_route" {
		t.Errorf("respawned wolf SuperPointName = %q, want the spawn's", got)
	}
}

// The life time counts the AI cycles since the NPC spawned and starts
// over at its death.
func TestNpcLifeTimeCountsAICycles(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithNPCs(memoTemplates()), gameservertest.WithNpcSpawns(memoDen(t)))

	wolf := liveOf(t, w.srv, memoWolfID).(*npc.Hostile)
	grocer := liveOf(t, w.srv, memoGrocerID).(*npc.Folk)
	if wolf.LifeTime() != 0 || grocer.LifeTime() != 0 {
		t.Fatalf("life times at spawn = %d, %d; want 0", wolf.LifeTime(), grocer.LifeTime())
	}
	for range 2 {
		if err := wolf.TickThink(); err != nil {
			t.Fatalf("wolf TickThink: %v", err)
		}
	}
	done := make(chan error, 1)
	grocer.Queue().Post(func() { done <- grocer.TickThink() })
	w.srv.Settle(t)
	if err := <-done; err != nil {
		t.Fatalf("grocer TickThink: %v", err)
	}
	if wolf.LifeTime() != 2 || grocer.LifeTime() != 1 {
		t.Fatalf("life times after the cycles = %d, %d; want 2, 1", wolf.LifeTime(), grocer.LifeTime())
	}
	if !wolf.Die(nil, nil) {
		t.Fatal("wolf did not die")
	}
	if got := wolf.LifeTime(); got != 0 {
		t.Fatalf("wolf life time after death = %d, want 0", got)
	}
}

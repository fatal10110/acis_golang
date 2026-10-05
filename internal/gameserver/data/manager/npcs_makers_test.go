package manager

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/rs/zerolog"
)

// A respawn that lands while its maker's spawn condition holds is deleted
// again by the created hook, and arms no further respawn: the default
// maker for a maker with an event attribute, the event maker for its
// EventName (DefaultMaker.onNpcCreated, EventMaker.onNpcCreated).
func TestMakerCreatedHookDeletesWhileHeld(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "events.xml"), eventsSpawnlist)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	f := newEventsFixture(t, table, eventsTemplates(), []string{"18age"})
	for _, tc := range []struct {
		npcID int
		key   string
	}{
		{2, "sellers#0#0"},    // event attribute
		{4, "orc_seller#0#0"}, // event_maker
	} {
		f.npcs.events = newSpawnEvents([]string{"18age"})
		f.hostile(t, tc.npcID).DeleteMe()
		f.queues.Run()
		if !f.respawn.Tracked(tc.key) {
			t.Fatalf("npc %d armed no respawn while listed", tc.npcID)
		}

		f.npcs.events = nil
		f.respawn.Cancel(tc.key)
		f.npcs.Respawn(tc.key)
		f.queues.Run()
		if slices.Contains(f.spawnedIDs(), tc.npcID) {
			t.Fatalf("npc %d stays in the world while its maker is held", tc.npcID)
		}
		if f.respawn.Tracked(tc.key) {
			t.Fatalf("npc %d deleted by its created hook armed a respawn", tc.npcID)
		}
	}
}

// A maker timer armed with Every runs on the makers' queue until its
// ticker is stopped; After runs once.
func TestMakerTimersRunOnTheMakersQueue(t *testing.T) {
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "events.xml"), eventsSpawnlist)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	f := newEventsFixture(t, table, eventsTemplates(), nil)
	g := f.npcs.currentGroups()[0]

	ticks, once := 0, 0
	ticker := g.Every(time.Second, func() { ticks++ })
	g.After(1500*time.Millisecond, func() { once++ })
	f.queues.Advance(3 * time.Second)
	if ticks != 3 || once != 1 {
		t.Fatalf("after 3s: ticks = %d, once = %d; want 3, 1", ticks, once)
	}
	ticker.Stop()
	f.queues.Advance(3 * time.Second)
	if ticks != 3 || once != 1 {
		t.Fatalf("after the stop: ticks = %d, once = %d; want 3, 1", ticks, once)
	}
}

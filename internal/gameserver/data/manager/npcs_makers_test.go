package manager

import (
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/rs/zerolog"
)

// A respawn that lands while its maker's spawn condition holds is deleted
// again by the created hook, and arms no further respawn
// (DefaultMaker.onNpcCreated). The Seven Signs event group stands through
// the competition; its NPC respawning after the state moved to seal
// validation, before the period change's pass runs, is held.
func TestMakerCreatedHookDeletesWhileHeld(t *testing.T) {
	f := newEventsFixture(t, eventsTable(t), eventsTemplates(), nil)
	ss := &fakeSevenSigns{period: sevensigns.Competition}
	f.npcs.StartSevenSigns(ss)
	const ssqID, key = 6, "ssq#0#0"
	f.hostile(t, ssqID).DeleteMe()
	f.queues.Run()
	if !f.respawn.Tracked(key) {
		t.Fatal("Seven Signs event NPC armed no respawn in the competition")
	}

	ss.period = sevensigns.SealValidation
	f.respawn.Cancel(key)
	f.npcs.Respawn(key)
	f.queues.Run()
	if slices.Contains(f.spawnedIDs(), ssqID) {
		t.Fatal("Seven Signs event NPC stays in the world at seal validation")
	}
	if f.respawn.Tracked(key) {
		t.Fatal("Seven Signs event NPC deleted by its created hook armed a respawn")
	}
}

// An event maker's NPC lives only while its EventName is listed
// (EventMaker.onNpcCreated, onNpcDeleted): listed, the NPC that leaves arms
// its respawn; unlisted, the NPC script event 1001 brings is deleted at
// once and arms none.
func TestEventMakerCreatedHookDeletesWhileUnlisted(t *testing.T) {
	const orcID, key = 4, "orc_seller#0#0"
	listed := newEventsFixture(t, eventsTable(t), eventsTemplates(), []string{"18age"})
	listed.hostile(t, orcID).DeleteMe()
	listed.queues.Run()
	if !listed.respawn.Tracked(key) {
		t.Fatal("listed event maker NPC armed no respawn")
	}

	unlisted := newEventsFixture(t, eventsTable(t), eventsTemplates(), nil)
	if slices.Contains(unlisted.spawnedIDs(), orcID) {
		t.Fatal("unlisted event maker spawned at boot")
	}
	unlisted.npcs.MakerEvent("orc_seller", "1001", 0, 0)
	unlisted.queues.Advance(time.Second)
	if slices.Contains(unlisted.spawnedIDs(), orcID) {
		t.Fatal("unlisted event maker NPC stays in the world after 1001")
	}
	if unlisted.respawn.Tracked(key) {
		t.Fatal("unlisted event maker NPC deleted by its created hook armed a respawn")
	}
}

// A respawn a maker schedules for an NPC that already has one pending does
// not push it back: of the two, the first to come due brings the NPC back
// (Npc.scheduleRespawn arms a task of its own each time). The default
// maker's NPC leaves with a 60s respawn; script event 1001 then asks for
// 600s, or for 10s.
func TestMakerScheduleRespawnKeepsTheEarlierDeadline(t *testing.T) {
	for _, tc := range []struct {
		name  string
		int1  int
		fires time.Duration
	}{
		{"later request", 600, 60 * time.Second},
		{"sooner request", 10, 10 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Unix(1000, 0)
			var mu sync.Mutex
			now := start
			clock := func() time.Time {
				mu.Lock()
				defer mu.Unlock()
				return now
			}
			set := func(d time.Duration) {
				mu.Lock()
				now = start.Add(d)
				mu.Unlock()
			}
			fired := &firedRespawns{}
			f := newEventsFixtureAt(t, eventsTable(t), eventsTemplates(), nil, clock, fired)
			const key = "plain#0#0"
			f.hostile(t, 1).DeleteMe()
			f.queues.Run()
			if !f.npcs.MakerEvent("plain", "1001", tc.int1, 0) {
				t.Fatal("maker plain not found")
			}

			set(tc.fires - time.Second)
			f.respawn.Tick()
			if got := fired.take(); len(got) != 0 {
				t.Fatalf("respawns 1s before %v = %v, want none", tc.fires, got)
			}
			set(tc.fires)
			f.respawn.Tick()
			if got := fired.take(); !slices.Equal(got, []string{key}) {
				t.Fatalf("respawns at %v = %v, want [%s]", tc.fires, got, key)
			}
		})
	}
}

// firedRespawns records the slot keys whose respawn came due.
type firedRespawns struct {
	mu   sync.Mutex
	keys []string
}

func (r *firedRespawns) Respawn(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys = append(r.keys, key)
}

func (r *firedRespawns) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.keys
	r.keys = nil
	return out
}

func eventsTable(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	writeSpawnFixture(t, filepath.Join(dir, "events.xml"), eventsSpawnlist)
	table, err := xml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist() error: %v", err)
	}
	return table
}

// A maker timer armed with Every runs on the makers' queue until its
// ticker is stopped; After runs once.
func TestMakerTimersRunOnTheMakersQueue(t *testing.T) {
	f := newEventsFixture(t, eventsTable(t), eventsTemplates(), nil)
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

package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestQuietRegionMonsterSeesStraySummon pins the tick scan of a monster
// that keeps ticking in a region no player keeps active, a noSleepMode
// one. Among NPCs alone it is told nothing. A summon whose owner stands
// beyond the monster's surrounding regions is still a playable it knows:
// it is told about it once, and again after the summon left its see range
// and came back. The owner walking up is told as well.
func TestQuietRegionMonsterSeesStraySummon(t *testing.T) {
	t.Parallel()
	calls := &seeCalls{names: map[int32]string{}}
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithAITask(),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{seeWatcherID: script.KindHostile},
			[]script.Listing{{Path: "ai.SeeWatcher"}},
			script.Catalog{"ai.SeeWatcher": func() script.Script {
				return script.Script{Behavior: true, NPCs: []int32{seeWatcherID}, Hooks: script.Hooks{OnSeeCreature: calls.see}}
			}}),
	})
	pet, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	far := location.Location{X: x + 12000, Y: y, Z: z}
	at := func(dx, dy int) location.Location { return location.Location{X: far.X + dx, Y: far.Y + dy, Z: far.Z} }

	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.ID, tmpl.TemplateID = int(seeWatcherID), int(seeWatcherID)
	tmpl.NoSleepMode = true
	watcher := h.srv.SpawnMovingHostileNPCTemplate(t, tmpl, far, far)
	calls.names[watcher.ObjectID()] = "watcher"
	h.srv.AI.Add(watcher)
	// More NPCs around it than a 32-entry known-list buffer holds.
	for i := range 40 {
		crowd := gameservertest.MovingHostileTemplate("Monster")
		h.srv.SpawnHostileNPCTemplateAt(t, crowd, at(-1500+(i%10)*300, -1500+(i/10)*300))
	}
	tick := func() {
		t.Helper()
		if err := h.srv.AI.Tick(); err != nil {
			t.Fatalf("AI.Tick() error: %v", err)
		}
		h.srv.Settle(t)
	}
	if h.srv.State.RegionActive(watcher) || watcher.AISleeping() {
		t.Fatalf("watcher region active = %v, sleeping = %v; want a quiet region and an awake monster",
			h.srv.State.RegionActive(watcher), watcher.AISleeping())
	}
	tick()
	calls.want(t, "among NPCs alone")

	move := func(o world.Tracked, what string, l location.Location) {
		t.Helper()
		if err := h.srv.State.Move(o, l.X, l.Y, l.Z); err != nil {
			t.Fatalf("move %s: %v", what, err)
		}
	}
	move(pet, "summon", at(100, 0))
	if h.srv.State.RegionActive(watcher) {
		t.Fatal("watcher region active with only the summon near it, want quiet")
	}
	tick()
	calls.want(t, "with the stray summon in range", "watcher summon")
	tick()
	calls.want(t, "on the next tick")

	move(pet, "summon", at(0, -700))
	tick()
	calls.want(t, "with the summon out of range")
	move(pet, "summon", at(100, 0))
	tick()
	calls.want(t, "with the summon back in range", "watcher summon")

	player, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from the world")
	}
	move(player, "owner", at(-100, 0))
	tick()
	calls.want(t, "with the owner walked up", "watcher owner")
}

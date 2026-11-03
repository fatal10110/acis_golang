package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Npc.onSpawn, onActiveRegion, onInactiveRegion, teleportTo,
// instantTeleportTo, doDie and deleteMe add the NPC to or remove it from
// AiTaskManager, which flips isAISleeping; AiTaskManager.run ticks every
// member with no region or territory test. The territory an NPC is tested
// against is a sphere of MaxDriftRange (200) around its spawn point.

// latchOff is a stand outside the 200 territory sphere around home.
const latchOff = 300

// latchedNPC is a hostile or civilian NPC under the AI-sleep latch.
type latchedNPC interface {
	world.Tracked
	AISleeping() bool
	LifeTime() int32
}

// bootSleepLatch boots a character in the world, the AI task driven by
// hand, and returns where it stands.
func bootSleepLatch(t *testing.T) (*gameservertest.Server, location.Location) {
	t.Helper()
	return bootFolkDesires(t, nil)
}

// spawnLatched spawns a moving monster homed at home, standing at at, and
// registers it with the AI task, as the spawn list does.
func spawnLatched(t *testing.T, srv *gameservertest.Server, tmpl *npc.Template, home, at location.Location) *npc.Hostile {
	t.Helper()
	h := srv.SpawnMovingHostileNPCTemplate(t, tmpl, home, at)
	srv.AI.Add(h)
	return h
}

// offset is at moved dx along x.
func offset(at location.Location, dx int) location.Location {
	return location.Location{X: at.X + dx, Y: at.Y, Z: at.Z}
}

// logOut takes the character out of the world and waits for the region
// around at to go quiet.
func logOut(t *testing.T, srv *gameservertest.Server, n world.Tracked) {
	t.Helper()
	srv.Client.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	srv.AdvanceUntil(t, "the region going quiet", func() bool {
		_, active := srv.State.RegionActivity(n)
		return !active
	})
	srv.Settle(t)
}

// onNPCQueue runs fn on q's owner's queue and waits for it.
func onNPCQueue(t *testing.T, srv *gameservertest.Server, h *npc.Hostile, fn func()) {
	t.Helper()
	if !h.Queue().Post(fn) {
		t.Fatal("the npc's queue is closed")
	}
	srv.Settle(t)
}

// assertLatch fails unless n's AISleeping is asleep and one AI task cycle
// ticks n exactly when it is awake.
func assertLatch(t *testing.T, srv *gameservertest.Server, n latchedNPC, asleep bool, what string) {
	t.Helper()
	if got := n.AISleeping(); got != asleep {
		t.Fatalf("%s: AISleeping() = %v, want %v", what, got, asleep)
	}
	before := n.LifeTime()
	if err := srv.AI.Tick(); err != nil {
		t.Fatalf("%s: AI.Tick() error: %v", what, err)
	}
	srv.Settle(t)
	ticked := n.LifeTime() != before
	if ticked == asleep {
		t.Fatalf("%s: AI task ticked = %v (LifeTime %d -> %d), want %v", what, ticked, before, n.LifeTime(), !asleep)
	}
}

// walkTo runs h's spawn cycle, then starts it walking to dest on its AI,
// which must be awake, and returns once the walk is under way.
func walkTo(t *testing.T, srv *gameservertest.Server, h *npc.Hostile, dest location.Location) {
	t.Helper()
	assertLatch(t, srv, h, false, "spawned in an active region")
	onNPCQueue(t, srv, h, func() {
		if !h.AI().AddMoveToDesire(dest, 1000) {
			t.Error("AddMoveToDesire() = false, want the walk queued")
		}
		if err := h.TickThink(); err != nil {
			t.Errorf("TickThink() error: %v", err)
		}
	})
	if got, moving := h.MovingTo(); !moving || got != dest {
		t.Fatalf("monster moving to %v (%v), want walking to %v", got, moving, dest)
	}
}

// finishWalk runs the movement task until h's walk ends.
func finishWalk(t *testing.T, srv *gameservertest.Server, h *npc.Hostile) {
	t.Helper()
	for range 400 {
		if !h.Move().Moving() {
			return
		}
		srv.TickPositions()
	}
	t.Fatal("the walk did not end within 400 movement ticks")
}

// TestOutOfTerritoryAtDeactivationKeepsTicking pins a monster out of its
// territory when its region goes quiet: it stays in the AI task, and walking
// back into its territory while the region stays quiet does not put it to
// sleep.
func TestOutOfTerritoryAtDeactivationKeepsTicking(t *testing.T) {
	t.Parallel()
	srv, at := bootSleepLatch(t)
	home := offset(at, 200)
	h := spawnLatched(t, srv, gameservertest.MovingHostileTemplate("Monster"), home, offset(home, latchOff))
	walkTo(t, srv, h, home)

	logOut(t, srv, h)
	assertLatch(t, srv, h, false, "out of territory as the region went quiet")

	finishWalk(t, srv, h)
	if !h.InTerritory() {
		t.Fatal("test setup: the monster is not back in its territory")
	}
	assertLatch(t, srv, h, false, "back home in the quiet region")
}

// TestInTerritoryAtDeactivationStaysAsleep pins a monster in its territory
// when its region goes quiet: it sleeps, and a walk under way ending outside
// its territory does not wake it.
func TestInTerritoryAtDeactivationStaysAsleep(t *testing.T) {
	t.Parallel()
	srv, at := bootSleepLatch(t)
	home := offset(at, 200)
	h := spawnLatched(t, srv, gameservertest.MovingHostileTemplate("Monster"), home, home)
	out := offset(home, latchOff)
	walkTo(t, srv, h, out)

	logOut(t, srv, h)
	assertLatch(t, srv, h, true, "in territory as the region went quiet")

	finishWalk(t, srv, h)
	if h.InTerritory() {
		t.Fatal("test setup: the monster is still in its territory")
	}
	assertLatch(t, srv, h, true, "outside its territory in the quiet region")
}

// TestSpawnInQuietRegionDecidesSleep pins the spawn rule in a region no
// player keeps active: a monster in its territory sleeps, one outside it and
// a noSleepMode one do not.
func TestSpawnInQuietRegionDecidesSleep(t *testing.T) {
	t.Parallel()
	srv, at := bootSleepLatch(t)
	far := offset(at, 40000)
	noSleep := gameservertest.MovingHostileTemplate("Monster")
	noSleep.NoSleepMode = true
	for _, tc := range []struct {
		name   string
		tmpl   *npc.Template
		dx     int
		asleep bool
	}{
		{"in territory", gameservertest.MovingHostileTemplate("Monster"), 0, true},
		{"out of territory", gameservertest.MovingHostileTemplate("Monster"), latchOff, false},
		{"noSleepMode", noSleep, 0, false},
	} {
		h := spawnLatched(t, srv, tc.tmpl, far, offset(far, tc.dx))
		if _, active := srv.State.RegionActivity(h); active {
			t.Fatal("test setup: the far region is active")
		}
		assertLatch(t, srv, h, tc.asleep, tc.name)
	}
}

// TestRegionWakeAndTeleportsWakeTheAI pins the wake points of a sleeping
// monster: its region waking, and a teleport landing it outside its
// territory; one landing inside keeps it asleep. Its region going quiet
// again puts it back to sleep.
func TestRegionWakeAndTeleportsWakeTheAI(t *testing.T) {
	t.Parallel()
	srv, at := bootSleepLatch(t)
	far := offset(at, 40000)
	h := spawnLatched(t, srv, gameservertest.MovingHostileTemplate("Monster"), far, far)
	assertLatch(t, srv, h, true, "spawned in territory in a quiet region")

	onNPCQueue(t, srv, h, func() { h.TeleportTo(offset(far, 50)) })
	assertLatch(t, srv, h, true, "teleported inside its territory")
	onNPCQueue(t, srv, h, func() { h.TeleportTo(offset(far, latchOff)) })
	assertLatch(t, srv, h, false, "teleported outside its territory")

	g := spawnLatched(t, srv, gameservertest.MovingHostileTemplate("Monster"), far, far)
	assertLatch(t, srv, g, true, "a second monster spawned in territory")
	player, ok := srv.State.Object(srv.SoleObjectID(t))
	if !ok {
		t.Fatal("the character is not in the world")
	}
	if err := srv.State.Teleport(player, far.X, far.Y, far.Z); err != nil {
		t.Fatalf("teleport the character: %v", err)
	}
	srv.Settle(t)
	assertLatch(t, srv, g, false, "its region woke")

	if err := srv.State.Teleport(player, at.X, at.Y, at.Z); err != nil {
		t.Fatalf("teleport the character back: %v", err)
	}
	srv.Settle(t)
	assertLatch(t, srv, g, true, "its region went quiet again")
}

// TestDeathAndPrivatesSleep pins death putting a monster to sleep in an
// active region, and a private out of its own territory following its
// master's, which is in its own, to sleep in a quiet region.
func TestDeathAndPrivatesSleep(t *testing.T) {
	t.Parallel()
	srv, at := bootSleepLatch(t)
	home := offset(at, 200)
	dying := spawnLatched(t, srv, gameservertest.MovingHostileTemplate("Monster"), home, home)
	assertLatch(t, srv, dying, false, "spawned in an active region")
	onNPCQueue(t, srv, dying, func() { dying.Die(nil, nil) })
	if got := dying.AISleeping(); !got {
		t.Fatal("dead monster: AISleeping() = false, want true")
	}

	master := spawnLatched(t, srv, gameservertest.MovingHostileTemplate("Monster"), home, home)
	private := spawnLatched(t, srv, gameservertest.MovingHostileTemplate("Monster"), offset(home, -latchOff), home)
	private.SetMaster(master)
	master.AddMinion(private)
	logOut(t, srv, master)
	assertLatch(t, srv, private, true, "private of a master in its territory")
}

// TestFolkSleepFollowsTheLatch pins the civilian NPC's spawn rule in a quiet
// region: one in its territory sleeps, a noSleepMode one keeps ticking.
func TestFolkSleepFollowsTheLatch(t *testing.T) {
	t.Parallel()
	srv, at := bootSleepLatch(t)
	far := offset(at, 40000)
	for _, tc := range []struct {
		name    string
		noSleep bool
		asleep  bool
	}{{"in territory", false, true}, {"noSleepMode", true, false}} {
		tmpl := gameservertest.FolkTemplate("Folk", 31000)
		tmpl.CanMove = true
		tmpl.NoSleepMode = tc.noSleep
		f := srv.SpawnFolkNPCAt(t, tmpl, far)
		srv.AI.Add(f)
		assertLatch(t, srv, f, tc.asleep, tc.name)
	}
}

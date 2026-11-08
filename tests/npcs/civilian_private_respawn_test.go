package npcs

import (
	"math"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// clerkTimer is the behavior timer the clerk starts when created.
const clerkTimer = "clerk_timer"

// clerkParty is a behavior bound to the tablet and the clerk. The clerk
// starts a timer when created. A party-died hook asks a dying private back
// as a party leader does: when its spawn declares a respawn delay and the
// hearing master lives, after that delay.
type clerkParty struct {
	self  atomic.Pointer[script.Script]
	fired atomic.Int32
}

func (c *clerkParty) script() script.Script {
	return script.Script{Behavior: true, NPCs: []int32{spawnTabletID, spawnClerkID}, Hooks: script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			c.self.Store(s)
			if e.NPC.NpcID() == spawnClerkID {
				s.StartTimer(clerkTimer, e.NPC, nil, time.Hour)
			}
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			if delay := s.Spawner().RespawnDelay(e.Caller); delay != 0 && e.Called.IsMaster() && !e.Called.Dead() {
				s.Spawner().ScheduleRespawn(e.Caller, time.Duration(delay)*time.Second)
			}
		},
		OnTimer: func(*script.Script, script.Timer) string {
			c.fired.Add(1)
			return ""
		},
	}}
}

// bootClerkParty boots the script spawn world over a mortal tablet that
// declares a mortal clerk private with a 30 s respawn delay, with c's
// behavior bound to both.
func bootClerkParty(t *testing.T, c *clerkParty) *scriptSpawnWorld {
	t.Helper()
	tablet := gameservertest.FolkTemplate("Folk", spawnTabletID)
	tablet.Undying = false
	tablet.Privates = []npc.PrivateEntry{{NpcID: spawnClerkID, Weight: 1, RespawnDelay: 30 * time.Second}}
	clerk := gameservertest.FolkTemplate("Folk", spawnClerkID)
	clerk.Undying = false
	kinds := map[int32]script.NPCKind{
		spawnWolfID: script.KindHostile, spawnTabletID: script.KindFolk, spawnClerkID: script.KindFolk,
	}
	return bootScriptSpawns(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{spawnMonster(spawnWolfID, "Wolf"), tablet, clerk})),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "ai.ClerkParty"}}, script.Catalog{"ai.ClerkParty": c.script}),
	)
}

// killFolk kills the civilian NPC f by a player's hit.
func (w *scriptSpawnWorld) killFolk(t *testing.T, f *npc.Folk) {
	t.Helper()
	p := w.liveObject(t, w.player)
	onPlayerQueue(t, w, func() { f.TakeDamage(1_000_000, p) })
	w.srv.Settle(t)
	if !f.Dead() {
		t.Fatalf("civilian %d did not die", f.ObjectID())
	}
}

// decayFolk decays the corpse of the civilian NPC f, telling its spawn.
func (w *scriptSpawnWorld) decayFolk(t *testing.T, f *npc.Folk) {
	t.Helper()
	onQueueOf(t, f, func() { f.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(f.ObjectID())) })
	w.srv.Settle(t)
}

// privateSlots returns how many script-private spawn slots are kept.
func (w *scriptSpawnWorld) privateSlots() int {
	n := 0
	for _, name := range w.srv.NpcSpawns.Slots() {
		if strings.HasPrefix(name, "private#") {
			n++
		}
	}
	return n
}

// liveClerks returns the living clerks in the world.
func (w *scriptSpawnWorld) liveClerks() []*npc.Folk {
	var out []*npc.Folk
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && f.Instance.Template.ID == spawnClerkID && !f.Dead() {
			out = append(out, f)
		}
	}
	return out
}

// A civilian master's party-died hook reads the respawn delay its civilian
// private's spawn declares and asks the dying private back after it. The
// request waits for the corpse: due before the decay it brings nothing
// back. Once the corpse decayed, still one of its master's privates, the
// clerk comes back around its live master, shown, again its private, as a
// new life the kept handle follows.
func TestScriptCivilianPrivateAskedBackAtItsDeath(t *testing.T) {
	t.Parallel()
	w := bootClerkParty(t, &clerkParty{})
	master := w.spawner.AddSpawn(spawnTabletID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	w.spawner.CreatePrivates(master)
	w.srv.Settle(t)
	tablet := spawnedNPC(t, w, spawnTabletID).(*npc.Folk)
	clerk := spawnedNPC(t, w, spawnClerkID).(*npc.Folk)
	handle := script.NPCOf(clerk)
	if got := w.spawner.RespawnDelay(handle); got != 30 {
		t.Fatalf("clerk respawn delay = %d, want its declared 30", got)
	}

	w.killFolk(t, clerk)
	if armed := w.armedPrivates(); len(armed) != 1 {
		t.Fatalf("respawns pending after the party-died hook = %q, want one", armed)
	}
	w.respawnArmed(t)
	if got := w.liveClerks(); len(got) != 0 {
		t.Fatalf("clerks after a request due before the decay = %d, want none", len(got))
	}

	w.spawner.ScheduleRespawn(handle, 5*time.Second)
	w.decayFolk(t, clerk)
	if clerk.Master() != npc.PartyMember(tablet) || !slices.Contains(tablet.Minions(), npc.PartyMember(clerk)) {
		t.Fatal("a decayed clerk keeping its respawn delay left its master's privates")
	}
	drainFrames(t, w.c)
	w.respawnArmed(t)
	frames := drainFrames(t, w.c)

	back := spawnedNPC(t, w, spawnClerkID).(*npc.Folk)
	if back.ObjectID() == clerk.ObjectID() || handle.Decayed() || handle.ObjectID() != back.ObjectID() {
		t.Fatalf("clerk back as %d (was %d), handle %d decayed %v; want a new life the handle follows",
			back.ObjectID(), clerk.ObjectID(), handle.ObjectID(), handle.Decayed())
	}
	assertPrivateOf(t, w, handle, back, tablet)
	tx, ty, _ := tablet.Position()
	cx, cy, _, _ := npcInfoAt(t, frames, back.ObjectID())
	if d := math.Hypot(float64(cx)-float64(tx), float64(cy)-float64(ty)); !inPrivateRing(d, 38, 116) {
		t.Fatalf("respawned clerk shown %.2f from its master; want 38 to 116", d)
	}
}

// The respawn request rules hold for a civilian private as for a hostile
// one. A request that is not positive asks nothing. A one-time clerk asked
// back at its death comes back, let go by its decay: spawned for its
// master yet no longer its private. One nobody asks back takes its slot
// with it at its decay, and a later request asks nothing. A request due
// once the master has died brings nothing back. A civilian NPC that is no
// private panics.
func TestScriptCivilianOneTimePrivateRespawnRules(t *testing.T) {
	t.Parallel()
	w := bootClerkParty(t, &clerkParty{})
	master := w.spawner.AddSpawn(spawnTabletID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	tablet := spawnedNPC(t, w, spawnTabletID).(*npc.Folk)

	asked := w.spawner.CreateOnePrivate(master, spawnClerkID, 0)
	clerk := spawnedNPC(t, w, spawnClerkID).(*npc.Folk)
	if got := w.spawner.RespawnDelay(asked); got != 0 {
		t.Fatalf("one-time clerk respawn delay = %d, want 0", got)
	}
	w.killFolk(t, clerk)
	w.spawner.ScheduleRespawn(asked, 0)
	w.spawner.ScheduleRespawn(asked, -time.Second)
	if armed := w.armedPrivates(); len(armed) != 0 {
		t.Fatalf("requests that are not positive armed %q", armed)
	}
	w.spawner.ScheduleRespawn(asked, 3*time.Second)
	w.decayFolk(t, clerk)
	if clerk.Master() != nil || slices.Contains(tablet.Minions(), npc.PartyMember(clerk)) {
		t.Fatal("the decayed one-time clerk is still its master's private")
	}
	w.respawnArmed(t)
	back := spawnedNPC(t, w, spawnClerkID).(*npc.Folk)
	if back.ObjectID() == clerk.ObjectID() || asked.ObjectID() != back.ObjectID() {
		t.Fatalf("asked clerk back as %d (was %d, handle %d), want a new life the handle follows", back.ObjectID(), clerk.ObjectID(), asked.ObjectID())
	}
	if back.Master() != nil || slices.Contains(tablet.Minions(), npc.PartyMember(back)) || back.SpawnMaster() != npc.PartyMember(tablet) {
		t.Fatalf("respawned one-time clerk: master %v, spawned for %v; want no master, spawned for the tablet", back.Master(), back.SpawnMaster())
	}

	slots := w.privateSlots()
	w.killFolk(t, back)
	w.decayFolk(t, back)
	if got := w.privateSlots(); got != slots-1 {
		t.Fatalf("private slots after the decay of a clerk nobody asked back = %d, want %d", got, slots-1)
	}
	w.spawner.ScheduleRespawn(asked, time.Second)
	if armed := w.armedPrivates(); len(armed) != 0 {
		t.Fatalf("a request after the decay of a clerk nobody asked back armed %q", armed)
	}

	w.spawner.CreateOnePrivate(master, spawnClerkID, 0)
	orphan := spawnedNPC(t, w, spawnClerkID).(*npc.Folk)
	w.killFolk(t, orphan)
	w.spawner.ScheduleRespawn(script.NPCOf(orphan), 3*time.Second)
	w.decayFolk(t, orphan)
	w.killFolk(t, tablet)
	w.respawnArmed(t)
	if got := w.liveClerks(); len(got) != 0 {
		t.Fatalf("clerks after the master died = %d, want none", len(got))
	}

	mustPanic(t, "a respawn request for a civilian that is no private", func() { w.spawner.ScheduleRespawn(master, time.Second) })
}

// A script's decayMe hides a civilian private: it leaves the world, its
// players see it go, with no decayed hook and no respawn, its behavior
// timer stops and never fires, and its slot goes with it, so a later
// request asks nothing. A second hide changes nothing.
func TestScriptDecayMeHidesCivilianPrivate(t *testing.T) {
	t.Parallel()
	c := &clerkParty{}
	w := bootClerkParty(t, c)
	master := w.spawner.AddSpawn(spawnTabletID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	w.spawner.CreatePrivates(master)
	w.srv.Settle(t)
	tablet := spawnedNPC(t, w, spawnTabletID).(*npc.Folk)
	clerk := spawnedNPC(t, w, spawnClerkID).(*npc.Folk)
	handle := script.NPCOf(clerk)
	s := c.self.Load()
	if s == nil || !s.HasTimer(clerkTimer, handle, nil) {
		t.Fatal("the clerk's created hook started no timer")
	}
	drainFrames(t, w.c)
	slots := w.privateSlots()

	onQueueOf(t, clerk, func() { s.Spawner().DecayMe(handle) })
	w.srv.Settle(t)
	if _, ok := w.srv.State.Object(clerk.ObjectID()); ok || !handle.Decayed() {
		t.Fatal("the hidden clerk is still in the world")
	}
	if frames := drainFrames(t, w.c); !deletedIn(frames, clerk.ObjectID()) {
		t.Fatalf("frames %x miss the hidden clerk's DeleteObject", opcodes(frames))
	}
	if s.HasTimer(clerkTimer, handle, nil) {
		t.Fatal("the hidden clerk's behavior timer is still pending")
	}
	if got := w.privateSlots(); got != slots-1 {
		t.Fatalf("private slots after the hide = %d, want %d", got, slots-1)
	}
	if tablet.Dead() {
		t.Fatal("hiding the private killed its master")
	}
	w.spawner.ScheduleRespawn(handle, time.Second)
	if armed := w.armedPrivates(); len(armed) != 0 {
		t.Fatalf("a request for the hidden clerk armed %q", armed)
	}
	s.Spawner().DecayMe(handle)
	w.srv.Advance(t, time.Hour)
	if n := c.fired.Load(); n != 0 {
		t.Fatalf("the hidden clerk's timer fired %d times", n)
	}
	if got := w.liveClerks(); len(got) != 0 {
		t.Fatalf("clerks after the hide = %d, want none", len(got))
	}
}

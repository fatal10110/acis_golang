package npcs

import (
	"fmt"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The civilian party fixtures: a mortal civilian master (a quest tablet)
// and a civilian private (a clerk), beside the script spawn fixtures.
const (
	spawnTabletID = 30010
	spawnClerkID  = 30011
)

// civilianPartyTemplates is the script spawn table with the tablet and the
// clerk.
func civilianPartyTemplates() *npc.Table {
	tablet := gameservertest.FolkTemplate("Folk", spawnTabletID)
	tablet.Undying = false
	return npc.NewTable([]*npc.Template{
		spawnMonster(spawnWolfID, "Wolf"),
		spawnMonster(spawnPupID, "Pup"),
		tablet,
		gameservertest.FolkTemplate("Folk", spawnClerkID),
	})
}

// partyLog records the party hooks of the behavior bound to the pup.
type partyLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *partyLog) add(format string, args ...any) {
	l.mu.Lock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
	l.mu.Unlock()
}

func (l *partyLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	got := l.lines
	l.lines = nil
	return got
}

// pupBehavior is a behavior bound to the pup that records its party hooks
// by the template ids of caller and called.
func (l *partyLog) pupBehavior() script.Script {
	return script.Script{Behavior: true, NPCs: []int32{spawnPupID}, Hooks: script.Hooks{
		OnPartyAttacked: func(_ *script.Script, e script.PartyAttacked) {
			l.add("PARTY_ATTACKED caller=%d called=%d damage=%d", e.Caller.NpcID(), e.Called.NpcID(), e.Damage)
		},
		OnPartyDied: func(_ *script.Script, e script.PartyDied) {
			l.add("PARTY_DIED caller=%d called=%d", e.Caller.NpcID(), e.Called.NpcID())
		},
	}}
}

// bootCivilianParty boots the script spawn world over the civilian party
// table, with log's behavior bound to the pup.
func bootCivilianParty(t *testing.T, log *partyLog, extra ...gameservertest.Option) *scriptSpawnWorld {
	t.Helper()
	kinds := map[int32]script.NPCKind{
		spawnWolfID: script.KindHostile, spawnPupID: script.KindHostile,
		spawnTabletID: script.KindFolk, spawnClerkID: script.KindFolk,
	}
	return bootScriptSpawns(t, append([]gameservertest.Option{
		gameservertest.WithNPCs(civilianPartyTemplates()),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "ai.PupRecorder"}},
			script.Catalog{"ai.PupRecorder": log.pupBehavior}),
	}, extra...)...)
}

// assertPrivateOf fails unless live is a linked private of master, placed
// by a private's spawn of master, whose handle names master as its master.
func assertPrivateOf(t *testing.T, w *scriptSpawnWorld, h *script.NPC, live, master npc.PartyMember) {
	t.Helper()
	if h == nil || h.Decayed() {
		t.Fatalf("private %d handle = %v, want a live one", live.NpcID(), h)
	}
	if live.Master() != master || !slices.Contains(master.Minions(), live) || !live.IsPrivate() {
		t.Fatalf("private %d is not linked to master %d", live.NpcID(), master.NpcID())
	}
	if rec, ok := w.srv.NpcSpawns.SpawnOf(live.ObjectID()); !ok || rec.MasterID != master.ObjectID() {
		t.Fatalf("private %d spawn = %+v (%v), want master %d's private", live.NpcID(), rec, ok, master.ObjectID())
	}
	if !script.Same(h.Master(), script.NPCOf(master)) || !h.HasMaster() {
		t.Fatalf("private %d handle master = %v, want %d", live.NpcID(), h.Master(), master.ObjectID())
	}
}

// A civilian master creates a hostile and a civilian private through the
// API: both are linked to it and shown, the hostile one at the asked point
// with its spawn parameters, the civilian one around the master; the
// handles are live. Deleting the master's spawn deletes both with it.
func TestScriptCivilianMasterKeepsPrivates(t *testing.T) {
	t.Parallel()
	w := bootCivilianParty(t, &partyLog{})
	master := w.spawner.AddSpawn(spawnTabletID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	tablet := spawnedNPC(t, w, spawnTabletID).(*npc.Folk)

	knight := w.spawner.CreateOnePrivateEx(master, spawnPupID, script.Loc{X: w.at.X + 80, Y: w.at.Y + 10, Z: w.at.Z, Heading: 500}, 0, script.Params{P1: 7, P2: 8, P3: 9})
	clerk := w.spawner.CreateOnePrivate(master, spawnClerkID, 0)
	pup := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)
	folk := spawnedNPC(t, w, spawnClerkID).(*npc.Folk)
	assertPrivateOf(t, w, knight, pup, tablet)
	assertPrivateOf(t, w, clerk, folk, tablet)
	if !master.IsMaster() || master.HasMaster() {
		t.Fatalf("tablet handle master %v, has master %v; want a master with none", master.IsMaster(), master.HasMaster())
	}
	s := pup.Scratch()
	if a, b, c := s.Int(npc.IntParam1), s.Int(npc.IntParam2), s.Int(npc.IntParam3); a != 7 || b != 8 || c != 9 {
		t.Fatalf("knight params = %d, %d, %d; want 7, 8, 9", a, b, c)
	}
	frames := drainFrames(t, w.c)
	if x, y, z, heading := npcInfoAt(t, frames, pup.ObjectID()); x != int32(w.at.X+80) || y != int32(w.at.Y+10) || z != int32(w.at.Z-groundBelow) || heading != 500 {
		t.Fatalf("knight shown at %d,%d,%d heading %d; want the asked point on the ground", x, y, z, heading)
	}
	tx, ty, _ := tablet.Position()
	cx, cy, _, _ := npcInfoAt(t, frames, folk.ObjectID())
	if d := math.Hypot(float64(cx)-float64(tx), float64(cy)-float64(ty)); !inPrivateRing(d, 38, 116) {
		t.Fatalf("clerk %.2f from its master; want 38 to 116", d)
	}
	if !folk.InTerritory() || !pup.InTerritory() {
		t.Fatal("a private of a civilian master is out of its territory")
	}

	if !w.srv.NpcSpawns.DeleteFixed(tablet.ObjectID()) {
		t.Fatal("the master's spawn was not deleted")
	}
	w.srv.Settle(t)
	if !master.Decayed() || !knight.Decayed() || !clerk.Decayed() {
		t.Fatalf("master gone %v, privates gone %v %v; want all", master.Decayed(), knight.Decayed(), clerk.Decayed())
	}
	frames = drainFrames(t, w.c)
	for _, id := range []int32{tablet.ObjectID(), pup.ObjectID(), folk.ObjectID()} {
		if !deletedIn(frames, id) {
			t.Fatalf("frames %x miss the DeleteObject of %d", opcodes(frames), id)
		}
	}
}

// A hostile master keeps a civilian private: it is linked, and once its
// despawn passes it is no longer the master's private. A civilian private
// left leaves the world with the master's spawn.
func TestScriptHostileMasterKeepsCivilianPrivate(t *testing.T) {
	t.Parallel()
	w := bootCivilianParty(t, &partyLog{})
	master := w.spawner.AddSpawn(spawnWolfID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	wolf := spawnedNPC(t, w, spawnWolfID).(*npc.Hostile)

	clerk := w.spawner.CreateOnePrivate(master, spawnClerkID, 10*time.Second)
	folk := spawnedNPC(t, w, spawnClerkID).(*npc.Folk)
	assertPrivateOf(t, w, clerk, folk, wolf)

	w.srv.Advance(t, 10*time.Second)
	if !clerk.Decayed() || slices.Contains(wolf.Minions(), npc.PartyMember(folk)) || folk.Master() != nil {
		t.Fatalf("despawned clerk gone %v, still a private %v", clerk.Decayed(), slices.Contains(wolf.Minions(), npc.PartyMember(folk)))
	}

	tablet := w.spawner.CreateOnePrivateEx(master, spawnTabletID, script.Loc{X: w.at.X + 90, Y: w.at.Y, Z: w.at.Z}, 0, script.Params{})
	assertPrivateOf(t, w, tablet, spawnedNPC(t, w, spawnTabletID).(*npc.Folk), wolf)
	if !w.srv.NpcSpawns.DeleteFixed(wolf.ObjectID()) {
		t.Fatal("the master's spawn was not deleted")
	}
	w.srv.Settle(t)
	if !master.Decayed() || !tablet.Decayed() {
		t.Fatalf("master gone %v, civilian private gone %v; want both", master.Decayed(), tablet.Decayed())
	}
}

// A civilian master's party: a hit or an offensive skill on the master
// calls its hostile private's party-attacked hook, and the master's death calls its
// party-died hook and lets the private go, which stays a private.
func TestScriptCivilianMasterPartyCalls(t *testing.T) {
	t.Parallel()
	log := &partyLog{}
	w := bootCivilianParty(t, log)
	master := w.spawner.AddSpawn(spawnTabletID, script.Loc{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}, false, 0)
	tablet := spawnedNPC(t, w, spawnTabletID).(*npc.Folk)
	knight := w.spawner.CreateOnePrivate(master, spawnPupID, 0)
	pup := spawnedNPC(t, w, spawnPupID).(*npc.Hostile)
	p := w.liveObject(t, w.player)

	onPlayerQueue(t, w, func() { tablet.TakeDamage(10, p) })
	onPlayerQueue(t, w, func() { tablet.SkillAttacked(p, skill.Definition{ID: 1160, Level: 1, Offensive: true, Debuff: true}) })
	onPlayerQueue(t, w, func() { tablet.TakeDamage(1_000_000, p) })
	w.srv.Settle(t)
	want := []string{
		fmt.Sprintf("PARTY_ATTACKED caller=%d called=%d damage=10", spawnTabletID, spawnPupID),
		fmt.Sprintf("PARTY_ATTACKED caller=%d called=%d damage=120", spawnTabletID, spawnPupID),
		fmt.Sprintf("PARTY_ATTACKED caller=%d called=%d damage=1000000", spawnTabletID, spawnPupID),
		fmt.Sprintf("PARTY_DIED caller=%d called=%d", spawnTabletID, spawnPupID),
	}
	if got := log.take(); !slices.Equal(got, want) {
		t.Fatalf("party hooks = %q, want %q", got, want)
	}
	if !tablet.Dead() || pup.Master() != nil || knight.HasMaster() {
		t.Fatalf("tablet dead %v, private still linked %v", tablet.Dead(), pup.Master() != nil)
	}
	if pup.SpawnMaster() != npc.PartyMember(tablet) || !pup.IsPrivate() || !pup.InTerritory() {
		t.Fatal("the master's death made the private a free spawn")
	}
}

// onPlayerQueue runs fn on the player's queue and waits for it.
func onPlayerQueue(t *testing.T, w *scriptSpawnWorld, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player).Post(func() {
		defer close(done)
		fn()
	}) {
		t.Fatal("the player's queue is closed")
	}
	<-done
}

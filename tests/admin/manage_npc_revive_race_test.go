package admin

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	// reviveRaceID is the monster the revive race maker spawns.
	reviveRaceID = 20120
	// reviveRaceSlot is the spawn slot of that monster.
	reviveRaceSlot = "revive_den#0#0"
)

// spawnDecay is a manualDecay whose corpse decays arm the decayed NPC's
// respawn, as the server's do.
type spawnDecay struct {
	*manualDecay
	srv *gameservertest.Server
}

func newSpawnDecay(t *testing.T) *spawnDecay {
	t.Helper()
	d := &spawnDecay{manualDecay: &manualDecay{now: time.Unix(1_700_000_000, 0)}}
	decay, err := task.NewDecay(d, d.clock)
	if err != nil {
		t.Fatalf("task.NewDecay: %v", err)
	}
	d.task = decay
	return d
}

func (d *spawnDecay) Decay(actor task.DecayActor) {
	d.mu.Lock()
	srv := d.srv
	d.mu.Unlock()
	obj, ok := srv.State.Object(actor.ObjectID())
	if !ok {
		return
	}
	if corpse, ok := obj.(interface {
		DecayWithRespawn(*world.State, func(int32) func()) bool
	}); ok {
		corpse.DecayWithRespawn(srv.State, srv.NpcSpawns.RespawnHook)
	}
}

// tickHeld moves the clock on by elapsed and runs one decay sweep without
// waiting for what it posted: the corpse's queue may be held.
func (d *spawnDecay) tickHeld(t *testing.T, elapsed time.Duration) {
	t.Helper()
	d.mu.Lock()
	d.now = d.now.Add(elapsed)
	d.mu.Unlock()
	if err := d.task.Tick(); err != nil {
		t.Fatalf("decay tick: %v", err)
	}
}

// reviveDen is a maker of one monster with a corpse time of
// reviveCorpseTime and a 60 second respawn.
func reviveDen(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	body := `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="den" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="revive_den" territory="den" maximumNpcs="1">
		<npc id="20120" total="1" pos="100;100;30;0" respawn="60sec"/>
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

// bootReviveRace boots a GM next to the reviveDen monster, selected and
// killed with //kill, with its corpse on decay's sweep.
func bootReviveRace(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, *spawnDecay, *npc.Hostile) {
	t.Helper()
	decay := newSpawnDecay(t)
	monster := &npc.Template{
		ID: reviveRaceID, TemplateID: reviveRaceID, Type: "Monster", Name: "Wolf", Level: 1, HPMax: 1000, MPMax: 100,
		CorpseTime: reviveCorpseTime, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{monster})),
		gameservertest.WithNpcSpawns(reviveDen(t)),
		gameservertest.WithDecay(decay.task),
		// The monster's queue is held while the GM's command runs; the
		// inline executor runs every queue on one goroutine.
		gameservertest.WithRealPool())
	decay.mu.Lock()
	decay.srv = srv
	decay.state = srv.State
	decay.mu.Unlock()
	gm := srv.Client
	enterWorld(t, gm)
	live := npcsOf(srv, reviveRaceID)
	if len(live) != 1 {
		t.Fatalf("monsters = %d, want the maker's one", len(live))
	}
	h := live[0].(*npc.Hostile)
	drain(t, gm)
	exchange(t, gm, encodeAction(h.ObjectID()))
	exchange(t, gm, encodeBuildCmd("kill"))
	onParaQueue(t, h.Queue(), func() {})
	if !h.Dead() || !decay.task.Tracked(h) {
		t.Fatalf("after //kill: dead %v, decay tracked %v, want both", h.Dead(), decay.task.Tracked(h))
	}
	drain(t, gm)
	return srv, gm, decay, h
}

// holdQueue holds h's queue until the returned release runs.
func holdQueue(t *testing.T, h *npc.Hostile) (release func()) {
	t.Helper()
	gate := make(chan struct{})
	if !h.Queue().Post(func() { <-gate }) {
		t.Fatal("post to queue: queue closed")
	}
	return func() { close(gate) }
}

// frameIndex returns the index of the first frame of opcode about id, or -1.
func frameIndex(frames [][]byte, opcode byte, id int32) int {
	for i, f := range frames {
		if len(f) >= 5 && f[0] == opcode && int32(binary.LittleEndian.Uint32(f[1:5])) == id {
			return i
		}
	}
	return -1
}

// requireDecayedOnce fails unless h has decayed out of the world, dead,
// with its spawn's respawn armed once and no living NPC left in its place,
// and then the respawn brings back exactly one.
func requireDecayedOnce(t *testing.T, srv *gameservertest.Server, h *npc.Hostile) {
	t.Helper()
	if _, ok := srv.State.Object(h.ObjectID()); ok || !h.Decayed() || !h.Dead() {
		t.Fatalf("monster in the world %v, decayed %v, dead %v; want it decayed out and dead", ok, h.Decayed(), h.Dead())
	}
	if n, live := srv.RespawnsArmed(), len(npcsOf(srv, reviveRaceID)); n != 1 || live != 0 || !srv.RespawnArmed(t, reviveRaceSlot) {
		t.Fatalf("respawns armed %d, live monsters %d; want its one respawn and none alive", n, live)
	}
	srv.RespawnSlot(t, reviveRaceSlot)
	srv.Settle(t)
	if live := npcsOf(srv, reviveRaceID); len(live) != 1 || live[0].ObjectID() == h.ObjectID() {
		t.Fatalf("live monsters after the respawn = %d, want one new one", len(live))
	}
}

// TestAdminManageMonsterResAheadOfDecay pins //res queued on a monster
// corpse ahead of the decay its corpse time has already posted: the res
// stands it up and its watcher sees it rise, then the decay takes it out of
// the world all the same, and its spawn respawns it once.
func TestAdminManageMonsterResAheadOfDecay(t *testing.T) {
	t.Parallel()
	srv, gm, decay, h := bootReviveRace(t)

	release := holdQueue(t, h)
	frames := exchange(t, gm, encodeBuildCmd("res"))
	decay.tickHeld(t, 2*reviveCorpseTime*time.Second)
	if decay.task.Tracked(h) {
		t.Fatal("decay still tracks the corpse after its deadline")
	}
	release()
	srv.Settle(t)
	frames = append(frames, settle(t, gm)...)

	revive, deleted := frameIndex(frames, serverpackets.OpcodeRevive, h.ObjectID()), frameIndex(frames, serverpackets.OpcodeDeleteObject, h.ObjectID())
	if revive < 0 || deleted < revive {
		t.Fatalf("frames = %x, want the Revive then the DeleteObject of %d", testsupport.FrameOpcodes(frames), h.ObjectID())
	}
	requireDecayedOnce(t, srv, h)
}

// TestAdminManageMonsterResBehindDecay pins //res queued on a monster
// corpse behind the decay its corpse time has already posted: the decay
// takes it out of the world, the res then finds nothing to stand up and
// tells the GM nothing, and its spawn respawns it once.
func TestAdminManageMonsterResBehindDecay(t *testing.T) {
	t.Parallel()
	srv, gm, decay, h := bootReviveRace(t)

	release := holdQueue(t, h)
	decay.tickHeld(t, 2*reviveCorpseTime*time.Second)
	frames := exchange(t, gm, encodeBuildCmd("res"))
	requireNoRevive(t, "//res behind the decay", frames)
	requireNoSystemMessage(t, "//res behind the decay", frames)
	release()
	srv.Settle(t)

	// What the GM then sees is the decay alone: its selection of the
	// monster is cleared (ActionFailed, TargetUnselected) and the monster
	// leaves its view.
	later := settle(t, gm)
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeTargetUnselected, serverpackets.OpcodeDeleteObject}
	if got := opcodesOf(later); !slices.Equal(got, want) {
		t.Fatalf("frames after the decay = %x, want %x: the decay's selection reset and DeleteObject, no Revive", got, want)
	}
	requireDecayedOnce(t, srv, h)
}

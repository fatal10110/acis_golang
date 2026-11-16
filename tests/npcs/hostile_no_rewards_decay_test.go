package npcs

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// noRewardGuardID is a town guard placed by an npcmaker: a hostile whose
// kill pays no reward.
const (
	noRewardGuardID  = 21301
	noRewardGuardKey = "norew_guard#0#0"
)

const noRewardSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="norew_guard" territory="field" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="21301" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

func noRewardTable(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "norew.xml"), []byte(noRewardSpawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

// spawnCorpses is a corpse-decay task on a clock the scenario moves by
// hand. Its effect is the server's: a due corpse still in the world decays
// there and arms its spawn's respawn.
type spawnCorpses struct {
	task *task.Decay

	mu  sync.Mutex
	now time.Time
	srv *gameservertest.Server
}

func newSpawnCorpses(t *testing.T) *spawnCorpses {
	t.Helper()
	d := &spawnCorpses{now: time.Unix(1_700_000_000, 0)}
	decay, err := task.NewDecay(d, d.clock)
	if err != nil {
		t.Fatalf("task.NewDecay: %v", err)
	}
	d.task = decay
	return d
}

func (d *spawnCorpses) clock() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.now
}

func (d *spawnCorpses) Decay(actor task.DecayActor) {
	d.mu.Lock()
	srv := d.srv
	d.mu.Unlock()
	if obj, ok := srv.State.Object(actor.ObjectID()); ok {
		if corpse, ok := obj.(interface {
			DecayWithRespawn(*world.State, func(int32) func()) bool
		}); ok {
			corpse.DecayWithRespawn(srv.State, srv.NpcSpawns.RespawnHook)
		}
	}
}

// passAndTick moves the clock on by elapsed and runs one decay sweep.
func (d *spawnCorpses) passAndTick(t *testing.T, srv *gameservertest.Server, elapsed time.Duration) {
	t.Helper()
	d.mu.Lock()
	d.now = d.now.Add(elapsed)
	d.srv = srv
	d.mu.Unlock()
	if err := d.task.Tick(); err != nil {
		t.Fatalf("decay tick: %v", err)
	}
	srv.Settle(t)
}

// TestHostileWithoutRewardsDecaysAndRespawns pins that a hostile NPC's
// corpse decay and its scripts' ground drops do not hang on its kill
// rewards. A town guard an npcmaker placed has none (its kill pays
// nothing), yet a script drop around it still falls, its corpse still
// leaves the world at its 7 s corpse time (Npc.doDie's
// DecayTaskManager.add), and its spawn brings it back.
func TestHostileWithoutRewardsDecaysAndRespawns(t *testing.T) {
	t.Parallel()
	guardTmpl := spawnMonster(noRewardGuardID, "Guard")
	guardTmpl.Type = "Guard"
	guardTmpl.CorpseTime = 7
	corpses := newSpawnCorpses(t)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{guardTmpl})),
		gameservertest.WithNpcSpawns(noRewardTable(t)),
		gameservertest.WithDecay(corpses.task),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, srv, c)
	drainUntilQuiet(t, c)
	guard := liveOf(t, srv, noRewardGuardID).(*npc.Hostile)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}

	// A script drop around the live guard falls on the ground.
	onQueueOf(t, guard, func() { guard.DropItem(player, item.AdenaID, 5) })
	srv.Settle(t)
	drops := srv.GroundItems.Snapshots(func(int32) bool { return false })
	if len(drops) != 1 || drops[0].TemplateID != item.AdenaID || drops[0].Count != 5 {
		t.Fatalf("ground items after the script drop = %+v, want one stack of 5 adena", drops)
	}
	if _, ok := firstOpcode(drainFrames(t, c), serverpackets.OpcodeDropItem); !ok {
		t.Fatal("the script drop sent no DropItem")
	}

	if !guard.TakeDamage(1_000_000, player) {
		t.Fatal("lethal hit did not kill the guard")
	}
	srv.Settle(t)
	if deadline, ok := guard.CorpseDeadline(); !ok || !deadline.Equal(corpses.clock().Add(7*time.Second)) {
		t.Fatalf("corpse deadline = %v (%v), want the 7 s corpse time", deadline, ok)
	}
	drainUntilQuiet(t, c)

	corpses.passAndTick(t, srv, 6*time.Second)
	if guard.Decayed() {
		t.Fatal("the corpse left before its corpse time")
	}
	corpses.passAndTick(t, srv, time.Second)
	if _, ok := srv.State.Object(guard.ObjectID()); ok || !guard.Decayed() {
		t.Fatal("the corpse did not decay at its corpse time")
	}
	if frameAt(drainFrames(t, c), serverpackets.OpcodeDeleteObject, guard.ObjectID()) < 0 {
		t.Fatal("the decay sent no DeleteObject")
	}
	if !srv.RespawnArmed(t, noRewardGuardKey) {
		t.Fatal("the decay armed no respawn")
	}

	srv.RespawnSlot(t, noRewardGuardKey)
	srv.Settle(t)
	back := liveOf(t, srv, noRewardGuardID).(*npc.Hostile)
	if back.ObjectID() == guard.ObjectID() || back.Dead() {
		t.Fatalf("respawned guard %d (was %d), dead %v; want a new live guard", back.ObjectID(), guard.ObjectID(), back.Dead())
	}
	if frameAt(drainFrames(t, c), serverpackets.OpcodeNPCInfo, back.ObjectID()) < 0 {
		t.Fatal("the respawn sent no NpcInfo")
	}
}

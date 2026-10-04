package admin

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// npcServerNotOperating is NPC_SERVER_NOT_OPERATING.
const npcServerNotOperating = 1278

// The slot keys of wolfDen's wolf and of its private.
const (
	denKey     = "wolf_den#0#0"
	privateKey = "wolf_den#0#0/private/0"
)

// staticsIn returns the ids of the parameterless system messages among
// frames.
func staticsIn(frames [][]byte) []int {
	var out []int
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(frame[1:])
		if id, params := r.ReadInt32(), r.ReadInt32(); params == 0 {
			out = append(out, int(id))
		}
	}
	return out
}

// noticesIn returns the plain-text system messages among frames, passing
// over the other system messages.
func noticesIn(frames [][]byte) []string {
	var out []string
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(frame[1:])
		if id, params := r.ReadInt32(), r.ReadInt32(); id == serverpackets.SystemMessageS1 && params == 1 && r.ReadInt32() == serverpackets.SystemMessageParamText {
			out = append(out, r.ReadString())
		}
	}
	return out
}

// allNpcs returns every NPC in the world.
func allNpcs(srv *gameservertest.Server) []int32 {
	var out []int32
	for _, obj := range srv.State.Objects() {
		switch obj.(type) {
		case *npc.Hostile, *npc.Folk, *npc.Decoration, *npc.EffectPoint:
			out = append(out, obj.ObjectID())
		}
	}
	return out
}

// denPrivate returns wolfDen's private.
func denPrivate(t *testing.T, srv *gameservertest.Server) *npc.Hostile {
	t.Helper()
	for _, obj := range npcsOf(srv, wolfID) {
		if rec, ok := srv.NpcSpawns.SpawnOf(obj.ObjectID()); ok && rec.MasterID != 0 {
			return obj.(*npc.Hostile)
		}
	}
	t.Fatal("wolf den has no private")
	return nil
}

// TestAdminUnspawnAll pins AdminSpawn.java's //unspawnall: every player is
// told NPC_SERVER_NOT_OPERATING (1278), every NPC leaves the world for good
// (the makers', the standalone ones, the decorations and the NPCs no spawn
// placed), a respawn already armed is cancelled, and every game master,
// hidden ones too, is told "NPCs' unspawn is now complete.".
func TestAdminUnspawnAll(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithNPCs(spawnTemplates()), gameservertest.WithNpcSpawns(wolfDen(t)), gameservertest.WithGMStartupUnlisted())
	gm := srv.Client
	enterWorld(t, gm)
	user, _ := addPlayer(t, srv, "user", "User", userLevel)
	hidden, _ := addPlayer(t, srv, "hidden", "Hidden", adminLevel)

	exchange(t, gm, encodeBuildCmd("spawn 30001"))
	exchange(t, gm, encodeBuildCmd("spawn 13006"))
	tmpl, _ := spawnTemplates().Get(wolfID)
	at := location.Location{X: spawnX + 60, Y: spawnY, Z: spawnZ}
	srv.SpawnMovingHostileNPCTemplate(t, tmpl, at, at)
	// The den's private leaves: its spawn waits to respawn.
	denPrivate(t, srv).DeleteMe()
	srv.Settle(t)
	if !srv.NpcRespawns.Tracked(privateKey) {
		t.Fatal("removed private armed no respawn")
	}
	if got := len(allNpcs(srv)); got != 4 {
		t.Fatalf("npcs in the world = %d, want the den wolf, the grocer, the tree and the loose wolf", got)
	}
	drain(t, user)
	drain(t, hidden)

	frames := exchange(t, gm, encodeBuildCmd("unspawnall"))
	if got := staticsIn(frames); !slices.Equal(got, []int{npcServerNotOperating}) {
		t.Fatalf("game master static messages = %v (frames %x), want NPC_SERVER_NOT_OPERATING", got, testsupport.FrameOpcodes(frames))
	}
	if got := noticesIn(frames); !slices.Equal(got, []string{"NPCs' unspawn is now complete."}) {
		t.Fatalf("game master messages = %q, want the unspawn notice", got)
	}
	userFrames := pending(t, user)
	if got := staticsIn(userFrames); !slices.Equal(got, []int{npcServerNotOperating}) {
		t.Fatalf("player static messages = %v, want NPC_SERVER_NOT_OPERATING", got)
	}
	if got := noticesIn(userFrames); len(got) != 0 {
		t.Fatalf("player told %q, want no game master notice", got)
	}
	if got := noticesIn(pending(t, hidden)); !slices.Equal(got, []string{"NPCs' unspawn is now complete."}) {
		t.Fatalf("hidden game master messages = %q, want the unspawn notice", got)
	}

	srv.Settle(t)
	if got := allNpcs(srv); len(got) != 0 {
		t.Fatalf("npcs left in the world: %v", got)
	}
	if got := srv.NpcSpawns.LiveCount(); got != 0 {
		t.Fatalf("live spawned npcs = %d, want 0", got)
	}
	for _, key := range []string{denKey, privateKey} {
		if srv.NpcRespawns.Tracked(key) {
			t.Fatalf("respawn of %s still armed", key)
		}
	}
}

// TestAdminUnspawnAllRacingRespawn runs //unspawnall while the den's
// respawn keeps firing: whichever lands first, no NPC is left and no
// respawn stays armed.
func TestAdminUnspawnAllRacingRespawn(t *testing.T) {
	t.Parallel()
	srv, _ := bootSpawnAdmin(t, wolfDen(t))
	gm := srv.Client
	den := npcsOf(srv, wolfID)[0].(*npc.Hostile)
	den.DeleteMe()
	srv.Settle(t)
	if !srv.NpcRespawns.Tracked(denKey) {
		t.Fatal("removed den wolf armed no respawn")
	}

	var stop atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Bounded, so the wolves it places stay within what one exchange
		// reads.
		for range 20 {
			if stop.Load() {
				return
			}
			srv.NpcSpawns.Respawn(denKey)
		}
	}()
	frames := exchange(t, gm, encodeBuildCmd("unspawnall"))
	stop.Store(true)
	<-done
	if got := noticesIn(frames); !slices.Equal(got, []string{"NPCs' unspawn is now complete."}) {
		t.Fatalf("game master messages = %q, want the unspawn notice", got)
	}

	srv.Settle(t)
	if got := allNpcs(srv); len(got) != 0 {
		t.Fatalf("npcs left in the world: %v", got)
	}
	if srv.NpcRespawns.Tracked(denKey) || srv.NpcSpawns.LiveCount() != 0 {
		t.Fatalf("respawn armed %v, live %d; want neither", srv.NpcRespawns.Tracked(denKey), srv.NpcSpawns.LiveCount())
	}
}

// newDen is the spawn list //respawnall reads in TestAdminRespawnAll: two
// wolves and a grocer.
func newDen(t *testing.T) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	body := `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="den" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="new_den" territory="den" maximumNpcs="3">
		<npc id="20120" total="2" pos="100;100;30;0" respawn="60sec"/>
		<npc id="30001" total="1" pos="150;150;30;0" respawn="60sec"/>
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

// TestAdminRespawnAll pins AdminSpawn.java's //respawnall: the NPCs leave as
// for //unspawnall, with nothing told to the players; the NPC templates and
// the spawn list are read again, the spawn list from the one in use; every
// on-start maker of the new list spawns, the standalone spawns staying
// gone; and every game master is told "NPCs' respawn is now complete.".
func TestAdminRespawnAll(t *testing.T) {
	t.Parallel()
	templates := spawnTemplates()
	fresh := newDen(t)
	var saved atomic.Pointer[gamemanager.Spawns]
	reloads := network.DataReloads{
		NPCs: func() error {
			templates.Replace(renamedWolfTemplates())
			return nil
		},
		SpawnList: func(_ context.Context, current *gamemanager.Spawns) (*gamemanager.Spawns, error) {
			saved.Store(current)
			return gamemanager.NewSpawns(fresh, nil), nil
		},
	}
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithNPCs(templates), gameservertest.WithNpcSpawns(wolfDen(t)), gameservertest.WithDataReloads(reloads))
	gm := srv.Client
	enterWorld(t, gm)
	user, _ := addPlayer(t, srv, "user", "User", userLevel)
	boot := srv.NpcSpawns.Spawns()
	exchange(t, gm, encodeBuildCmd("spawn 30001"))
	standalone := npcsOf(srv, grocerID)
	if len(standalone) != 1 {
		t.Fatalf("grocers = %d, want the standalone one", len(standalone))
	}
	drain(t, user)

	frames := exchange(t, gm, encodeBuildCmd("respawnall"))
	if got := noticesIn(frames); !slices.Equal(got, []string{"NPCs' respawn is now complete."}) {
		t.Fatalf("game master messages = %q, want the respawn notice", got)
	}
	if got := staticsIn(frames); slices.Contains(got, npcServerNotOperating) {
		t.Fatal("game master told NPC_SERVER_NOT_OPERATING")
	}
	userFrames := pending(t, user)
	if got := staticsIn(userFrames); slices.Contains(got, npcServerNotOperating) {
		t.Fatal("player told NPC_SERVER_NOT_OPERATING")
	}
	if got := noticesIn(userFrames); len(got) != 0 {
		t.Fatalf("player told %q, want nothing", got)
	}

	srv.Settle(t)
	if saved.Load() != boot {
		t.Fatal("the spawn list was not reloaded from the one in use")
	}
	if srv.NpcSpawns.Spawns().Table() != fresh {
		t.Fatal("the reloaded spawn list is not in use")
	}
	wolves := npcsOf(srv, wolfID)
	if len(wolves) != 2 {
		t.Fatalf("wolves = %d, want the new den's 2", len(wolves))
	}
	for _, w := range wolves {
		if name := w.(*npc.Hostile).Instance.Name(); name != "Dire Wolf" {
			t.Fatalf("respawned wolf is %q, want the reloaded template", name)
		}
	}
	grocers := npcsOf(srv, grocerID)
	if len(grocers) != 1 || grocers[0].ObjectID() == standalone[0].ObjectID() {
		t.Fatalf("grocers = %d, want only the new den's", len(grocers))
	}
	if rec, ok := srv.NpcSpawns.SpawnOf(grocers[0].ObjectID()); !ok || rec.Maker != "new_den" {
		t.Fatalf("grocer spawn = %+v, %v; want the new den's maker", rec, ok)
	}
	if got := srv.NpcSpawns.LiveCount(); got != 3 {
		t.Fatalf("live spawned npcs = %d, want 3", got)
	}
}

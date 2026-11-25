package ai

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group/randomteleport"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// mammonDen is an npcmaker spawning one Merchant of Mammon, whose respawn
// the test drives by hand.
const mammonDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="mammon_den" minZ="-4000" maxZ="-2000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="mammon_den" territory="mammon_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="31113" total="1" pos="40;20;-3000;0" respawn="600min"/>
	</npcmaker>
</list>`

// mammonDenKey is the slot key of the den's Merchant.
const mammonDenKey = "mammon_den#0#0"

// TestMammonMovesNoMoreOutOfTheWorld pins #5135: once the Merchant of Mammon
// leaves the world, his half-hourly move stops, drawing nothing and moving
// nobody, so the place the next Merchant must not repeat is still the one
// the last live Merchant moved to. A Merchant respawned before his
// predecessor's next move moves half an hour after he appears.
func TestMammonMovesNoMoreOutOfTheWorld(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(mammonDen), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	draws := &listedDraws{draws: []int{
		7,    // the first Merchant's move: the last place
		7, 2, // the third Merchant's move: where the first went, then the third place
	}}
	const path = "script.ai.group.RandomTeleport"
	catalog := script.Catalog{path: randomteleport.New}
	srv, _ := bootFolkDesires(t, catalog,
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{merchantOfMammonID: script.KindFolk},
			[]script.Listing{{Path: path}}, catalog),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{gameservertest.FolkTemplate("SignsPriest", int(merchantOfMammonID))})),
		gameservertest.WithNpcSpawns(table),
		gameservertest.WithScriptRand(draws.draw),
	)

	merchant := func() *npc.Folk {
		t.Helper()
		var live []*npc.Folk
		for _, obj := range srv.State.Objects() {
			if f, ok := obj.(*npc.Folk); ok && int32(f.NpcID()) == merchantOfMammonID && !f.Decayed() {
				live = append(live, f)
			}
		}
		if len(live) != 1 {
			t.Fatalf("live Merchants = %d, want 1", len(live))
		}
		return live[0]
	}
	movedTo := func(step string, f *npc.Folk, wantDraws []string, dest location.Location) {
		t.Helper()
		if got := draws.take(); !slices.Equal(got, wantDraws) {
			t.Fatalf("%s: draws = %q, want %q", step, got, wantDraws)
		}
		if x, y, _ := f.Position(); x != dest.X || y != dest.Y {
			t.Fatalf("%s: the Merchant stands at %d,%d, want %d,%d", step, x, y, dest.X, dest.Y)
		}
	}
	stays := func(step string) {
		t.Helper()
		if got := draws.take(); len(got) != 0 {
			t.Fatalf("%s: draws = %q, want none", step, got)
		}
		for _, f := range srv.ReadQueued(t, srv.Client) {
			if f[0] == serverpackets.OpcodeTeleportToLocation {
				t.Fatalf("%s: a TeleportToLocation was sent, want none", step)
			}
		}
	}
	leave := func(f *npc.Folk) {
		t.Helper()
		done := make(chan struct{})
		if !f.Queue().Post(func() {
			defer close(done)
			f.Kill(nil)
			f.Decay(srv.State, srv.NpcSpawns.RespawnHook(f.ObjectID()))
		}) {
			t.Fatal("the Merchant's queue is closed")
		}
		<-done
		srv.Settle(t)
		if !f.Decayed() {
			t.Fatal("the Merchant is still in the world after his deletion")
		}
	}

	first := merchant()
	srv.Advance(t, 30*time.Minute)
	movedTo("the first Merchant's move", first, []string{"8:7"}, location.Location{X: 172384, Y: -17823})
	srv.ReadQueued(t, srv.Client)

	srv.Advance(t, 10*time.Minute)
	leave(first)
	srv.Advance(t, 2*time.Hour)
	stays("four half hours after the first Merchant left")

	srv.RespawnSlot(t, mammonDenKey)
	srv.Settle(t)
	second := merchant()
	srv.Advance(t, 10*time.Minute)
	leave(second)
	srv.RespawnSlot(t, mammonDenKey)
	srv.Settle(t)
	third := merchant()
	srv.Advance(t, 20*time.Minute)
	stays("when the second Merchant would have moved")

	srv.Advance(t, 10*time.Minute)
	movedTo("the third Merchant's move", third, []string{"8:7", "8:2"}, location.Location{X: 83136, Y: 208992})
	if n := draws.left(); n != 0 {
		t.Fatalf("%d listed draws never drawn", n)
	}
}

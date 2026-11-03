package admin

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

const (
	morphDenNPCID = 100
	morphDenKey   = "morph_den#0#0"
)

// TestAdminPolymorphSurvivesRespawn pins the form of a spawned monster
// across its respawn (ASpawn.doRespawn reuses the one Npc, and only
// //polymorph writes its form): the monster it brings back is shown in the
// form the last one left in, to players already around and from its first
// NpcInfo; one that left as itself comes back as itself.
func TestAdminPolymorphSurvivesRespawn(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="morph_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="morph_den" territory="morph_field" maximumNpcs="1">
		<npc id="100" total="1" pos="50;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`
	if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	makers, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	own := &npc.Template{
		ID: morphDenNPCID, TemplateID: morphDenNPCID, Type: "Monster", Name: "Den Wolf", Level: 1, HPMax: 100,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20, RightHand: 7, LeftHand: 8,
		UsingServerSideName: true,
	}
	templates := npc.NewTable([]*npc.Template{own, {
		ID: polymorphNPCID, TemplateID: polymorphNPCID + 1, Name: polymorphForm.Name,
		RightHand: int(polymorphForm.RightHand), LeftHand: int(polymorphForm.LeftHand),
		CollisionRadius: polymorphForm.CollisionRadius, CollisionHeight: polymorphForm.CollisionHeight,
	}})
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithNPCs(templates), gameservertest.WithNpcSpawns(makers))
	gm := srv.Client
	enterWorld(t, gm)
	watcher, _ := addPlayer(t, srv, "player2", "Watcher", userLevel)
	settle(t, watcher)

	itself := npcLook{
		template: morphDenNPCID, radius: own.CollisionRadius, height: own.CollisionHeight,
		rightHand: int32(own.RightHand), left: int32(own.LeftHand), name: own.Name,
	}
	form := npcLook{
		template: polymorphNPCID + 1, radius: polymorphForm.CollisionRadius, height: polymorphForm.CollisionHeight,
		rightHand: polymorphForm.RightHand, left: polymorphForm.LeftHand, name: own.Name,
	}
	live := func(step string) *npc.Hostile {
		t.Helper()
		got := npcsOf(srv, morphDenNPCID)
		if len(got) != 1 {
			t.Fatalf("%s: den NPCs in the world = %d, want 1", step, len(got))
		}
		return got[0].(*npc.Hostile)
	}
	// respawn removes the den's NPC and brings the next one back, and
	// returns it with the look the watcher's first NpcInfo of it shows.
	respawn := func(step string, gone *npc.Hostile) (*npc.Hostile, npcLook) {
		t.Helper()
		gone.DeleteMe()
		srv.Settle(t)
		settle(t, watcher)
		if !srv.CancelRespawn(t, morphDenKey) {
			t.Fatalf("%s: no respawn armed for the den", step)
		}
		srv.RespawnSlot(t, morphDenKey)
		srv.Settle(t)
		back := live(step)
		if back.ObjectID() == gone.ObjectID() {
			t.Fatalf("%s: the den's NPC did not come back anew", step)
		}
		frames := settle(t, watcher)
		for _, f := range frames {
			if f[0] == serverpackets.OpcodeNPCInfo && frameObjectID(f) == back.ObjectID() {
				return back, npcInfoLook(f)
			}
		}
		t.Fatalf("%s: watcher frames %x show no NpcInfo of the respawned NPC", step, testsupport.FrameOpcodes(frames))
		return nil, npcLook{}
	}

	first := live("boot")
	exchange(t, gm, encodeAction(first.ObjectID()))
	exchange(t, gm, encodeBuildCmd("polymorph "+strconv.Itoa(polymorphNPCID)))
	settle(t, watcher)

	back, look := respawn("polymorphed", first)
	if look != form {
		t.Fatalf("watcher sees the respawned polymorphed NPC as %+v, want %+v", look, form)
	}
	if got := back.Polymorph(); got == nil || got.ID != polymorphNPCID {
		t.Fatalf("respawned NPC form = %+v, want NPC %d", got, polymorphNPCID)
	}

	// Shown as itself again, it comes back as itself.
	exchange(t, gm, encodeAction(back.ObjectID()))
	exchange(t, gm, encodeBuildCmd("polymorph"))
	settle(t, watcher)
	if back.Polymorph() != nil {
		t.Fatal("the toggle left the respawned NPC in its form")
	}
	again, look := respawn("unpolymorphed", back)
	if look != itself {
		t.Fatalf("watcher sees the respawned unpolymorphed NPC as %+v, want %+v", look, itself)
	}
	if again.Polymorph() != nil {
		t.Fatalf("respawned NPC form = %+v, want none", again.Polymorph())
	}
}

package admin

import (
	"bytes"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestAdminPolymorphSpawnProtectionEnds pins a spawn-protected GM's
// //polymorph on itself: the command is an action, so the protection ends
// after it (onActionRequest, Player.setSpawnProtection), and that refresh
// (Player.broadcastUserInfo) shows the players around the GM's NPC form
// again, never its CharInfo.
func TestAdminPolymorphSpawnProtectionEnds(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, polymorphNPCs(), gameservertest.WithSpawnProtection(time.Minute))
	gm := srv.Client
	enterWorld(t, gm)
	watcher, _ := addPlayer(t, srv, "player2", "Watcher", userLevel)
	settle(t, watcher)
	if !onlineCharacter(t, srv, gmID).SpawnProtected() {
		t.Fatal("GM entered the world without spawn protection")
	}

	frames := exchange(t, gm, encodeBuildCmd("polymorph "+strconv.Itoa(polymorphNPCID)))
	requirePolymorph(t, srv, gmID, polymorphNPCID)
	if onlineCharacter(t, srv, gmID).SpawnProtected() {
		t.Fatal("//polymorph left spawn protection on")
	}
	assertTexts(t, slices.DeleteFunc(slices.Clone(frames), func(f []byte) bool {
		return f[0] != serverpackets.OpcodeSystemMessage
	}), actedNotice)

	want := serverpackets.FramePcMorphInfo(onlineCharacter(t, srv, gmID), polymorphForm).Bytes()[2:]
	var shown [][]byte
	for _, f := range settle(t, watcher) {
		switch f[0] {
		case serverpackets.OpcodeCharInfo:
			if id, _ := charInfoHidden(t, f); id == gmID {
				t.Fatal("Watcher got the polymorphed GM's CharInfo")
			}
		case serverpackets.OpcodeNPCInfo:
			if frameObjectID(f) == gmID {
				shown = append(shown, f)
			}
		}
	}
	// One NPC form from the spawn, one from the protection refresh.
	if len(shown) != 2 {
		t.Fatalf("Watcher saw the GM's NPC form %d times, want 2", len(shown))
	}
	if !bytes.Equal(shown[1], want) {
		t.Fatalf("refreshed PcMorphInfo = %x, want %x", shown[1], want)
	}
}

// TestAdminPolymorphNPCTarget pins //polymorph on a selected NPC. The id is
// read first: none or an unreadable one gets the usage, one no NPC has is
// refused (APPLICANT_INFORMATION_INCORRECT). An NPC form for an NPC is not
// implemented yet, so a known id logs the gap and answers ActionFailed
// alone, leaving the NPC as it was for everyone around it.
//
// A working control tower takes the toggle-off branch whatever follows;
// a tower only works during a siege, whose castle, clans and siege start
// tests/clan builds, so that branch is covered with the NPC form itself.
func TestAdminPolymorphNPCTarget(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, polymorphNPCs())
	gm := srv.Client
	enterWorld(t, gm)
	monster := srv.SpawnHostileNPCAt(t, location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
	watcher, _ := addPlayer(t, srv, "player2", "Watcher", userLevel)
	exchange(t, gm, encodeAction(monster.ObjectID()))
	settle(t, watcher)

	frames := exchange(t, gm, encodeBuildCmd("polymorph "+strconv.Itoa(polymorphNPCID)))
	if got := testsupport.FrameOpcodes(frames); !bytes.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("known id on an NPC frames = %x, want ActionFailed alone", got)
	}
	for _, f := range settle(t, watcher) {
		if (f[0] == serverpackets.OpcodeNPCInfo || f[0] == serverpackets.OpcodeDeleteObject) && frameObjectID(f) == monster.ObjectID() {
			t.Fatalf("Watcher frame %x shows the NPC changed", f)
		}
	}

	frames = exchange(t, gm, encodeBuildCmd("polymorph 99999"))
	if len(frames) != 1 {
		t.Fatalf("unknown id on an NPC frames = %x, want one message", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageApplicantInformationIncorrect)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("polymorph")), adminPolymorphUsageText)
	requirePolymorph(t, srv, gmID, 0)
}

// TestAdminPolymorphDoorRefused pins //polymorph on a selected door: only
// an NPC or a player takes a form, so a known id is refused, and a missing
// id still gets the usage.
func TestAdminPolymorphDoorRefused(t *testing.T) {
	t.Parallel()
	gate := &door.Template{
		ID: infoGateID, Name: "info_gate", Kind: door.KindDoor, Level: 3,
		Position: location.Location{X: 100, Y: spawnY, Z: spawnZ},
		Coordinates: []location.Point{
			{X: 92, Y: spawnY - 8}, {X: 108, Y: spawnY - 8}, {X: 108, Y: spawnY + 8}, {X: 92, Y: spawnY + 8},
		},
		HP: 500, PDef: 100, MDef: 50, Height: 32,
	}
	srv, gmID := bootAdmin(t, adminLevel, polymorphNPCs(), gameservertest.WithDoors(gate))
	gm := srv.Client
	enterWorld(t, gm)
	d, ok := srv.WorldObjects.Door(infoGateID)
	if !ok {
		t.Fatal("gate not spawned")
	}
	exchange(t, gm, encodeAction(d.ObjectID()))

	frames := exchange(t, gm, encodeBuildCmd("polymorph "+strconv.Itoa(polymorphNPCID)))
	if len(frames) != 1 {
		t.Fatalf("door frames = %x, want one message", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageApplicantInformationIncorrect)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("polymorph")), adminPolymorphUsageText)
	requirePolymorph(t, srv, gmID, 0)
}

// adminPolymorphUsageText is what //polymorph answers a line it cannot read.
const adminPolymorphUsageText = "Usage: //polymorph npcId"

package admin

import (
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestAdminEffectsDoor pins the effects commands on a selected door: a
// door shows no abnormal state, so //abnormal sends nothing; //social
// shows the door's action to the players that know it; //effect set lands
// nothing and opens the page with no rows; //effect visual shows the door
// casting on the GM.
func TestAdminEffectsDoor(t *testing.T) {
	t.Parallel()
	gate := &door.Template{
		ID: infoGateID, Name: "info_gate", Kind: door.KindDoor, Level: 3,
		Position: location.Location{X: 100, Y: spawnY, Z: spawnZ},
		Coordinates: []location.Point{
			{X: 92, Y: spawnY - 8}, {X: 108, Y: spawnY - 8}, {X: 108, Y: spawnY + 8}, {X: 92, Y: spawnY + 8},
		},
		HP: 500, PDef: 100, MDef: 50, Height: 32,
	}
	srv, gmID := bootAdmin(t, adminLevel, effectsSkills(t), gameservertest.WithDoors(gate),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "char_effects.htm")))
	gm := srv.Client
	enterWorld(t, gm)
	d, ok := srv.WorldObjects.Door(infoGateID)
	if !ok {
		t.Fatal("gate not spawned")
	}
	exchange(t, gm, encodeAction(d.ObjectID()))
	drain(t, gm)
	run := func(command string) [][]byte {
		t.Helper()
		return append(exchange(t, gm, encodeBuildCmd(command)), settle(t, gm)...)
	}

	if frames := run("abnormal 1"); len(frames) != 0 {
		t.Fatalf("//abnormal on a door frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	requireOnly(t, "//social on a door", run("social 2"), socialBytes(d.ObjectID(), 2))

	page := requireEffectReply(t, "//effect set on a door", run("effect set "+strconv.Itoa(effectsBuff)+" 1"), nil, "admin_debug")
	if strings.Contains(page, "<td width=220>") {
		t.Fatalf("//effect set on a door page = %q, want no rows", page)
	}
	if again := requireEffectReply(t, "//effect on a door", run("effect"), nil, "admin_debug"); strings.Contains(again, "<td width=220>") {
		t.Fatalf("door effect page after //effect set = %q, want no rows", again)
	}

	dx, dy, dz := d.Position()
	gx, gy, gz := srv.PlayerPosition(t, gmID)
	if frames := run("effect visual 1020"); !holdsFrame(frames, magicSkillUseBytes(d.ObjectID(), gmID, dx, dy, dz, gx, gy, gz, effectsBuff)) {
		t.Fatalf("//effect visual on a door frames = %x, want the door's cast on the GM", frames)
	}
}

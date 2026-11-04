package admin

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// preyHeading is the heading the chased player faces once in the world.
const preyHeading = 12345

// addPreyAt enters the world at x, y, z, facing preyHeading, a player
// without access, returning its client and its live combatant.
func addPreyAt(t *testing.T, srv *gameservertest.Server, x, y, z int) (*testsupport.ScriptedClient, attackable.Combatant) {
	t.Helper()
	ch := srv.SeedCharacterFor(t, "dbgprey", "Prey", 1, 0)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = ?, y = ?, z = ?, heading = ? WHERE obj_Id = ?", x, y, z, preyHeading, ch.ID); err != nil {
		t.Fatal(err)
	}
	c := srv.DialClient(t, "dbgprey", 1)
	enterWorld(t, c)
	obj, ok := srv.State.Player(ch.ID)
	if !ok {
		t.Fatal("prey missing from the world")
	}
	live, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("prey %T is no combatant", obj)
	}
	return c, live
}

// headedText writes x, y, z and heading as a debug text prints a creature's
// position.
func headedText(x, y, z, heading int) string {
	return itoa(x) + ", " + itoa(y) + ", " + itoa(z) + ", " + itoa(heading)
}

// TestAdminShowMoveDebugsNPCChase pins //show move on a monster: its chase
// after a player is told to the game master as "Moving from <x, y, z,
// heading> to <x, y, z, heading>", both positions carrying the heading
// their creature faces (the monster's before it turns), and drawn on
// MOVE<monster id> with the first step: the click white, labelled with the
// player's position and heading, the start a red point, the straight walk
// a yellow "No geopath" line, the step a red point labelled with the
// heading the monster then faces. The chased player, no game master, gets
// none of it.
func TestAdminShowMoveDebugsNPCChase(t *testing.T) {
	t.Parallel()
	srv, gm, _ := bootGeoAdmin(t)
	prey, preyLive := addPreyAt(t, srv, spawnX, spawnY, geoZ)
	at := location.Location{X: spawnX - 600, Y: spawnY, Z: geoZ}
	monster := srv.SpawnMovingHostileNPCAt(t, "Monster", at, at)
	monsterID := monster.ObjectID()
	drain(t, gm)
	gm.Send(encodeAction(monsterID))
	drain(t, gm)
	frames := exchangeZones(t, gm, encodeBuildCmd("show move"))
	if len(frames) != 1 {
		t.Fatalf("//show move on the monster sent %d frames, want one text", len(frames))
	}
	if _, text := systemText(t, frames[0]); !strings.HasPrefix(text, "Debug move enabled on ") {
		t.Fatalf("//show move on the monster said %q", text)
	}
	drain(t, prey)

	turnedFrom := monster.Heading()
	monster.AddAttackDesire(preyLive, 50)
	if !monster.IsMoving() {
		t.Fatal("monster did not start chasing the player")
	}
	click := headedText(spawnX, spawnY, geoZ, preyHeading)
	_, texts := debugFrames(t, exchangeSettled(t, gm, nil))
	if want := "Moving from " + headedText(at.X, at.Y, at.Z, turnedFrom) + " to " + click; len(texts) != 1 || texts[0] != want {
		t.Fatalf("texts on the chase = %q, want %q", texts, want)
	}

	srv.TickPositions()
	drawings, _ := debugFrames(t, exchangeSettled(t, gm, nil))
	moveName := "MOVE" + strconv.Itoa(int(monsterID))
	if len(drawings) == 0 || drawings[0].name != moveName {
		t.Fatalf("drawings on the first step = %+v, want %s", drawings, moveName)
	}
	stepHeading := strconv.Itoa(monster.Heading())
	labels := map[string]int{}
	for _, s := range drawings[0].strokes {
		switch {
		case s.line && s.rgb == showWhite && s.label == "MoveToLocation: "+click && [3]int32(s.at[:3]) == [3]int32{int32(at.X), int32(at.Y), int32(at.Z)}:
			labels["click"]++
		case !s.line && s.rgb == showRed && s.label == "" && [3]int32(s.at[:3]) == [3]int32{int32(at.X), int32(at.Y), int32(at.Z)}:
			labels["start"]++
		case s.line && s.rgb == showYellow && s.label == "No geopath":
			labels["route"]++
		case !s.line && s.rgb == showRed && s.label == stepHeading:
			labels["step"]++
		}
	}
	if labels["click"] != 1 || labels["start"] != 1 || labels["route"] != 1 || labels["step"] == 0 {
		t.Fatalf("%s strokes %+v, want the click, the start, the route and a step facing %s", moveName, drawings[0].strokes, stepHeading)
	}

	srv.TickPositions()
	if drawings, texts := debugFrames(t, exchangeSettled(t, prey, nil)); len(drawings) != 0 || len(texts) != 0 {
		t.Fatalf("chased player got %+v %q, want no debug", drawings, texts)
	}
}

// TestAdminShowMoveDebugsPawnWalk pins the debug of a game master's own
// walk to attack a monster: told as "Moving from x y z to x y z", and drawn
// on MOVE<id> with the first step: the start a red point and the walk a
// white line labelled "MoveToPawn (<offset>): x y z".
func TestAdminShowMoveDebugsPawnWalk(t *testing.T) {
	t.Parallel()
	srv, gm, _ := bootGeoAdmin(t)
	gmID := srv.SoleObjectID(t)
	at := location.Location{X: spawnX - 300, Y: spawnY, Z: geoZ}
	monster := srv.SpawnHostileNPCAt(t, at)
	drain(t, gm)
	gm.Send(encodeAction(gmID))
	drain(t, gm)
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("show move")), "Debug move enabled on Admin")

	gm.Send(encodeAction(monster.ObjectID()))
	drain(t, gm)
	target := itoa(at.X) + " " + itoa(at.Y) + " " + itoa(at.Z)
	_, texts := debugFrames(t, exchangeSettled(t, gm, encodeAction(monster.ObjectID())))
	if want := "Moving from " + itoa(spawnX) + " " + itoa(spawnY) + " " + itoa(geoZ) + " to " + target; len(texts) != 1 || texts[0] != want {
		t.Fatalf("texts on the attack walk = %q, want %q", texts, want)
	}

	srv.TickPositions()
	drawings, _ := debugFrames(t, exchangeSettled(t, gm, nil))
	moveName := "MOVE" + strconv.Itoa(int(gmID))
	if len(drawings) == 0 || drawings[0].name != moveName || len(drawings[0].strokes) < 3 {
		t.Fatalf("drawings on the first step = %+v, want %s", drawings, moveName)
	}
	// The frame carries every point, in order, then every line.
	var start, walk stroke
	for i := len(drawings[0].strokes) - 1; i >= 0; i-- {
		if s := drawings[0].strokes[i]; s.line {
			walk = s
		} else {
			start = s
		}
	}
	if start.line || start.rgb != showRed || start.label != "" || [3]int32(start.at[:3]) != [3]int32{spawnX, spawnY, geoZ} {
		t.Fatalf("first point %+v, want the red start", start)
	}
	label := regexp.MustCompile(`^MoveToPawn \(\d+\): ` + target + `$`)
	if !walk.line || walk.rgb != showWhite || !label.MatchString(walk.label) || walk.at != [6]int32{spawnX, spawnY, geoZ, int32(at.X), int32(at.Y), int32(at.Z)} {
		t.Fatalf("first line %+v, want the white walk %q", walk, label)
	}
}

// TestAdminShowMoveDebugsFolkWalk pins //show move on a civilian NPC
// walking its route: the leg it starts on reaching the first node is told
// to the game master as "Moving from <x, y, z, heading> to <x, y, z>", the
// node a bare position.
func TestAdminShowMoveDebugsFolkWalk(t *testing.T) {
	t.Parallel()
	srv, gm, _ := bootGeoAdmin(t)
	const alias = "dbgwalker"
	tmpl := gameservertest.FolkTemplate("Folk", 30001)
	tmpl.Alias = alias
	tmpl.CanMove = true
	at := location.Location{X: spawnX - 300, Y: spawnY, Z: geoZ}
	first := location.Location{X: at.X - 50, Y: at.Y, Z: at.Z}
	second := location.Location{X: at.X - 450, Y: at.Y, Z: at.Z}
	folk, _ := srv.SpawnRouteFolkNPCAt(t, tmpl, at, route.WalkerRoutes{alias: {alias: {{Location: first}, {Location: second}}}}, false)
	drain(t, gm)
	gm.Send(encodeAction(folk.ObjectID()))
	drain(t, gm)
	assertTexts(t, exchangeZones(t, gm, encodeBuildCmd("show move")), "Debug move enabled on Folk")

	moving := regexp.MustCompile(`^Moving from ` + itoa(first.X) + `, ` + itoa(first.Y) + `, ` + itoa(first.Z) + `, \d+ to ` + itoa(second.X) + `, ` + itoa(second.Y) + `, ` + itoa(second.Z) + `$`)
	moveName := "MOVE" + strconv.Itoa(int(folk.ObjectID()))
	for range 20 {
		srv.TickPositions()
		drawings, texts := debugFrames(t, exchangeSettled(t, gm, nil))
		for _, d := range drawings {
			if d.name != moveName {
				t.Fatalf("drawing %s on the walk, want %s", d.name, moveName)
			}
		}
		if len(texts) == 0 {
			continue
		}
		if len(texts) != 1 || !moving.MatchString(texts[0]) {
			t.Fatalf("texts on the walk = %q, want %q", texts, moving)
		}
		return
	}
	t.Fatal("the walker never started its second leg")
}

package boat

import (
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/boat"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// revivedHP is a boat's HP after //res: the harness's RespawnRestoreHP of
// its maximum.
const revivedHP = boat.MaxHP * 0.7

// vitalsLog is reloadLog with the deaths and revivals c was shown.
func vitalsLog(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient) []string {
	t.Helper()
	var out []string
	for _, f := range srv.ReadQueued(t, c) {
		switch f[0] {
		case serverpackets.OpcodeDie:
			out = append(out, fmt.Sprintf("die %d", wire.NewReader(f[1:]).ReadInt32()))
			continue
		case serverpackets.OpcodeRevive:
			out = append(out, fmt.Sprintf("revive %d", wire.NewReader(f[1:]).ReadInt32()))
			continue
		}
		if s, ok := describe(f); ok {
			out = append(out, s)
			continue
		}
		if s, ok := describePassenger(f); ok {
			out = append(out, s)
		}
	}
	return out
}

func boatPosition(b *boat.Boat) location.Location {
	x, y, z := b.Position()
	return location.Location{X: x, Y: y, Z: z}
}

func revived(objectID int32) string { return fmt.Sprintf("revive %d", objectID) }

func died(objectID int32) string { return fmt.Sprintf("die %d", objectID) }

// TestAdminKillBreaksBoatUnderWay pins //kill over a radius that holds a
// boat under way: the boat stops where it stands, shown only by the
// vehicle-stop pair, with no death packet. The leg it was on never ends, so
// it sails no further and its schedule never moves on; its passengers stay
// aboard and its fare still falls due. //res mends it, shown by a revive, at
// the configured share of its HP, and it stays where it broke.
func TestAdminKillBreaksBoatUnderWay(t *testing.T) {
	t.Parallel()
	var fail atomic.Bool
	srv, gm, me := bootReload(t, nil, &fail, passengerItinerary())
	b := srv.Boats.Boats()[0]
	id := b.ObjectID()
	watcher, _ := addRider(t, srv, "watcher", "Watcher", runeBoarding)
	board(t, srv, gm, id)

	// Three seconds into the first leg, before the fare falls due.
	sail(srv, 303)
	if !b.Moving() {
		t.Fatal("boat not under way")
	}
	srv.ReadQueued(t, gm)
	srv.ReadQueued(t, watcher)

	// The game master aboard stands where the boat does, the dock far off.
	gm.Send(encodeBuildCmd("kill 100"))
	stoppedAt := boatPosition(b)
	stopped := []string{started(id, 0), info(id, stoppedAt, b.Heading())}
	assertLog(t, "game master aboard", vitalsLog(t, srv, gm), stopped...)
	assertLog(t, "dock", vitalsLog(t, srv, watcher), stopped...)
	if !b.Dead() || b.HP() != 0 || b.Moving() {
		t.Fatalf("after //kill: dead %v, HP %v, moving %v; want dead at 0 HP, stopped", b.Dead(), b.HP(), b.Moving())
	}

	sail(srv, 600)
	assertLog(t, "broken boat", vitalsLog(t, srv, watcher))
	if b.Moving() || boatPosition(b) != stoppedAt {
		t.Fatalf("broken boat moved: at %v, moving %v; want stopped at %v", boatPosition(b), b.Moving(), stoppedAt)
	}
	if !slices.Contains(b.Passengers(), me) {
		t.Fatalf("passengers %v, want the game master still aboard", b.Passengers())
	}
	if n := srv.PlayerItemCount(t, me, ticketID); n != 0 {
		t.Fatalf("game master holds %d tickets, want the fare taken", n)
	}
	srv.ReadQueued(t, gm)

	// A second //kill finds the boat broken already.
	gm.Send(encodeBuildCmd("kill 100"))
	assertLog(t, "kill again", vitalsLog(t, srv, gm))

	gm.Send(encodeBuildCmd("res 100"))
	assertLog(t, "game master aboard, //res", vitalsLog(t, srv, gm), revived(id))
	assertLog(t, "dock, //res", vitalsLog(t, srv, watcher), revived(id))
	if b.Dead() || b.HP() != revivedHP {
		t.Fatalf("after //res: dead %v, HP %v; want alive at %v", b.Dead(), b.HP(), revivedHP)
	}

	sail(srv, 600)
	assertLog(t, "mended boat", vitalsLog(t, srv, watcher))
	if b.Moving() || boatPosition(b) != stoppedAt {
		t.Fatalf("mended boat moved: at %v, moving %v; want stopped at %v", boatPosition(b), b.Moving(), stoppedAt)
	}

	// A living boat takes //res as nothing.
	gm.Send(encodeBuildCmd("res 100"))
	assertLog(t, "res a living boat", vitalsLog(t, srv, gm))
}

// TestAdminKillBreaksDockedBoat pins //heal, //cancel, //kill and //res over
// a radius that holds a boat tied up at its dock and a player: //heal and
// //cancel show nothing of the boat; //kill kills the player and breaks the
// boat, which keeps its schedule and puts out on time all the same; //heal
// leaves a broken boat broken; //res mends it under way. //reload boat still
// takes a broken boat out of the world.
func TestAdminKillBreaksDockedBoat(t *testing.T) {
	t.Parallel()
	var fail atomic.Bool
	srv, gm, _ := bootReload(t, nil, &fail, passengerItinerary())
	b := srv.Boats.Boats()[0]
	id := b.ObjectID()
	rider, riderID := addRider(t, srv, "rider", "Rider", runeShore)
	srv.ReadQueued(t, gm)
	srv.ReadQueued(t, rider)

	gm.Send(encodeBuildCmd("heal 1000"))
	gm.Send(encodeBuildCmd("cancel 1000"))
	assertLog(t, "heal and cancel", vitalsLog(t, srv, gm))
	if b.Dead() || b.HP() != boat.MaxHP {
		t.Fatalf("after //heal: dead %v, HP %v; want alive at %v", b.Dead(), b.HP(), boat.MaxHP)
	}

	gm.Send(encodeBuildCmd("kill 1000"))
	tiedUp := boatPosition(b)
	stopped := []string{started(id, 0), info(id, tiedUp, b.Heading())}
	got := vitalsLog(t, srv, gm)
	// The player dies on its own queue, so its death may come either side of
	// the boat's stop.
	if !slices.Contains(got, died(riderID)) {
		t.Fatalf("game master log %q lacks the player's death", got)
	}
	assertLog(t, "kill", slices.DeleteFunc(got, func(s string) bool {
		return !strings.HasPrefix(s, "started ") && !strings.HasPrefix(s, "info ")
	}), stopped...)
	if !b.Dead() || b.HP() != 0 {
		t.Fatalf("after //kill: dead %v, HP %v; want dead at 0 HP", b.Dead(), b.HP())
	}

	gm.Send(encodeBuildCmd("heal 1000"))
	srv.ReadQueued(t, gm)
	if !b.Dead() || b.HP() != 0 {
		t.Fatalf("after //heal: dead %v, HP %v; want still dead at 0 HP", b.Dead(), b.HP())
	}

	// Broken, it still puts out on its schedule.
	sail(srv, 303)
	if !b.Moving() || !b.Dead() {
		t.Fatalf("broken boat at its departure: moving %v, dead %v; want under way, dead", b.Moving(), b.Dead())
	}
	srv.ReadQueued(t, gm)

	gm.Send(encodeBuildCmd("res 1000"))
	got = vitalsLog(t, srv, gm)
	if !slices.Contains(got, revived(id)) {
		t.Fatalf("game master log %q lacks the boat's revive", got)
	}
	if b.Dead() || b.HP() != revivedHP || !b.Moving() {
		t.Fatalf("after //res: dead %v, HP %v, moving %v; want alive at %v, under way", b.Dead(), b.HP(), b.Moving(), revivedHP)
	}

	gm.Send(encodeBuildCmd("kill 1000"))
	srv.ReadQueued(t, gm)
	if !b.Dead() {
		t.Fatal("the mended boat did not break again")
	}
	gm.Send(encodeBuildCmd("reload boat"))
	srv.ReadQueued(t, gm)
	if _, ok := srv.State.Object(id); ok {
		t.Fatalf("broken boat %d still in the world after //reload boat", id)
	}
}

// TestAdminKillBoatDuringFleetTick breaks a boat under way while its fleet
// ticks on another goroutine: the break never interleaves with a position
// update or a leg's end, so the boat ends broken and stopped.
func TestAdminKillBoatDuringFleetTick(t *testing.T) {
	t.Parallel()
	var fail atomic.Bool
	srv, gm, _ := bootReload(t, nil, &fail, passengerItinerary())
	b := srv.Boats.Boats()[0]
	sail(srv, 303)
	srv.ReadQueued(t, gm)

	done := make(chan struct{})
	go func() {
		defer close(done)
		sail(srv, 4)
	}()
	gm.Send(encodeBuildCmd("kill 5000"))
	srv.ReadQueued(t, gm)
	<-done
	if !b.Dead() || b.Moving() {
		t.Fatalf("dead %v, moving %v; want broken and stopped", b.Dead(), b.Moving())
	}
}

// TestAdminKillBoatWithoutDamage pins //kill over a boat by a game master
// whose access level gives no damage: the hit takes nothing, so the boat
// keeps its full HP and nobody is shown it stop.
func TestAdminKillBoatWithoutDamage(t *testing.T) {
	t.Parallel()
	var fail atomic.Bool
	srv, gm, me := bootReload(t, nil, &fail, passengerItinerary())
	b := srv.Boats.Boats()[0]
	watcher, _ := addRider(t, srv, "watcher", "Watcher", runeBoarding)
	board(t, srv, gm, b.ObjectID())
	sail(srv, 303)
	srv.ReadQueued(t, gm)
	srv.ReadQueued(t, watcher)

	obj, ok := srv.State.Player(me)
	if !ok {
		t.Fatalf("world.Player(%d) missing", me)
	}
	denied, ok := obj.(interface{ SetCanGiveDamage(bool) })
	if !ok {
		t.Fatalf("world.Player(%d) = %T has no SetCanGiveDamage", me, obj)
	}
	denied.SetCanGiveDamage(false)

	gm.Send(encodeBuildCmd("kill 100"))
	assertLog(t, "game master aboard", vitalsLog(t, srv, gm))
	assertLog(t, "dock", vitalsLog(t, srv, watcher))
	if b.Dead() || b.HP() != boat.MaxHP || !b.Moving() {
		t.Fatalf("after a damageless //kill: dead %v, HP %v, moving %v; want alive at %v, under way", b.Dead(), b.HP(), b.Moving(), boat.MaxHP)
	}
}

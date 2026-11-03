package boat

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// gmLevel is the shipped access level of a game master allowed //reload.
const gmLevel = 7

// reloadTown is the town restart point the passengers of a reloaded fleet
// are put ashore at, out of sight of the Rune dock.
var reloadTown = location.Location{X: 43835, Y: -47749, Z: -792}

// reloadTownTable sends a player anywhere around the Rune dock to
// reloadTown.
func reloadTownTable() *restart.Table {
	var regions []location.Point
	for _, x := range []int{runeDock.X - 3000, runeDock.X + 3000} {
		regions = append(regions, location.Point{
			X: (x-world.MinX)/world.TileSize + world.TileXMin,
			Y: (runeDock.Y-world.MinY)/world.TileSize + world.TileYMin,
		})
	}
	return &restart.Table{Points: []restart.Point{{
		Name: "ReloadTown", Points: []location.Location{reloadTown}, ChaoPoints: []location.Location{reloadTown}, MapRegions: regions,
	}}}
}

// reloadedItinerary is the round trip the reload reads: the passenger
// itinerary heading the other way round.
func reloadedItinerary() route.BoatItinerary {
	it := passengerItinerary()
	it.Heading = 1000
	return it
}

// bootReload boots the game master Sailor at runeBoarding with one ticket,
// the passenger itinerary's boat tied up at Rune, the boat routes reading
// as routes (failing while fail is set), and boats as the fleet's boats.
func bootReload(t *testing.T, routes []route.BoatItinerary, fail *atomic.Bool, boats ...route.BoatItinerary) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	return bootReloadWith(t, routes, fail, boats)
}

// bootReloadWith is bootReload with extra boot options.
func bootReloadWith(t *testing.T, routes []route.BoatItinerary, fail *atomic.Bool, boats []route.BoatItinerary, opts ...gameservertest.Option) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	adminData, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	reloads := network.DataReloads{BoatRoutes: func() ([]route.BoatItinerary, error) {
		if fail.Load() {
			return nil, errors.New("broken boatRoutes.xml")
		}
		return routes, nil
	}}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Sailor", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithBoats(boats...),
		gameservertest.WithAdmin(adminData),
		gameservertest.WithRestartPoints(reloadTownTable()),
		gameservertest.WithDataReloads(reloads),
	}, opts...)...)
	me := srv.SoleObjectID(t)
	place(t, srv, me, runeBoarding)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = ? WHERE obj_Id = ?", gmLevel, me); err != nil {
		t.Fatalf("set access level: %v", err)
	}
	srv.GiveItem(t, me, ticketID, 1)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readUntilQuiet(c)
	return srv, c, me
}

// addRider enters a second character, name, standing at at with one
// ticket.
func addRider(t *testing.T, srv *gameservertest.Server, account, name string, at location.Location) (*testsupport.ScriptedClient, int32) {
	t.Helper()
	ch := srv.SeedCharacterFor(t, account, name, 1, 0)
	place(t, srv, ch.ID, at)
	srv.GiveItem(t, ch.ID, ticketID, 1)
	c := srv.DialClient(t, account, 1)
	c.Send(encodeRequestGameStart(0))
	c.Send(encodeEnterWorld())
	readUntilQuiet(c)
	return c, ch.ID
}

func place(t *testing.T, srv *gameservertest.Server, objID int32, at location.Location) {
	t.Helper()
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?", at.X, at.Y, at.Z, objID); err != nil {
		t.Fatalf("place character: %v", err)
	}
}

// reloadLog renders the boat, passenger and text frames c has been sent.
func reloadLog(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient) []string {
	t.Helper()
	var out []string
	for _, f := range srv.ReadQueued(t, c) {
		if f[0] == serverpackets.OpcodeSystemMessage {
			r := wire.NewReader(f[1:])
			if id := r.ReadInt32(); id == serverpackets.SystemMessageS1 && r.ReadInt32() == 1 && r.ReadInt32() == serverpackets.SystemMessageParamText {
				out = append(out, "text "+r.ReadString())
				continue
			}
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

func encodeBuildCmd(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	w.WriteString(command)
	return w.Bytes()
}

func text(s string) string { return "text " + s }

// reloadUsage is //reload's usage, answered to a reload that fails.
var reloadUsage = []string{
	text("Usage : //reload <admin|announcement|buylist|config>"),
	text("Usage : //reload <crest|cw|door|htm|item|multisell|npc>"),
	text("Usage : //reload <npcwalker|script|skill|teleport|zone>"),
}

// reloadDone is what the game master is told once the boats reloaded.
var reloadDone = text("Boat have been reloaded.")

// TestAdminReloadBoat pins //reload boat (BoatData.reload): every boat
// stops where it stands, showing so, and puts its passengers ashore at
// their town restart point, the game master among them at once, its pending
// fare dropped; once they are ashore it leaves the world, and the boats read
// anew put out from their docks. The game master is told the boats
// reloaded. A boatRoutes.xml that cannot be read leaves the fleet as it was.
func TestAdminReloadBoat(t *testing.T) {
	t.Parallel()
	var fail atomic.Bool
	srv, gm, me := bootReload(t, []route.BoatItinerary{reloadedItinerary()}, &fail, passengerItinerary())
	oldBoat := srv.Boats.Boats()[0]
	old := oldBoat.ObjectID()
	rider, riderID := addRider(t, srv, "rider", "Rider", runeBoarding)
	watcher, watcherID := addRider(t, srv, "watcher", "Watcher", runeBoarding)
	board(t, srv, gm, old)
	board(t, srv, rider, old)

	// Under way, its fare due five seconds after the departure.
	sail(srv, 305)
	for _, c := range []*testsupport.ScriptedClient{gm, rider, watcher} {
		srv.ReadQueued(t, c)
	}

	fail.Store(true)
	gm.Send(encodeBuildCmd("reload boat"))
	assertLog(t, "failed reload", reloadLog(t, srv, gm), reloadUsage...)
	if got := srv.Boats.Boats(); len(got) != 1 || got[0] != oldBoat || !oldBoat.Moving() {
		t.Fatal("a failed reload replaced or stopped the fleet")
	}
	fail.Store(false)

	x, y, z := oldBoat.Position()
	stoppedAt := location.Location{X: x, Y: y, Z: z}
	stopped := []string{started(old, 0), info(old, stoppedAt, oldBoat.Heading())}
	gm.Send(encodeBuildCmd("reload boats"))
	// The teleport ashore acks as every teleport does, and forgets the boat
	// and those still around it.
	assertLog(t, "game master aboard", reloadLog(t, srv, gm), append(stopped,
		af, af, af, af, af, teleport(me, reloadTown), deleted(old), deleted(watcherID), deleted(riderID),
		sm(serverpackets.SystemMessageExitPeacefulZone), reloadDone)...)
	assertLog(t, "passenger", reloadLog(t, srv, rider), append(stopped,
		teleport(me, reloadTown), deleted(me),
		af, af, af, af, af, teleport(riderID, reloadTown), deleted(old), deleted(watcherID),
		sm(serverpackets.SystemMessageExitPeacefulZone))...)
	if len(srv.Boats.Boats()) != 1 {
		t.Fatalf("fleet after the reload = %d boats, want 1", len(srv.Boats.Boats()))
	}
	newBoat := srv.Boats.Boats()[0]
	// On the dock, the old boat goes only once its passengers are ashore,
	// and the new one comes after it.
	assertLog(t, "dock", reloadLog(t, srv, watcher), append(stopped,
		teleport(me, reloadTown), deleted(me), teleport(riderID, reloadTown), deleted(riderID),
		deleted(old), info(newBoat.ObjectID(), runeDock, 1000))...)

	if _, ok := srv.State.Object(old); ok {
		t.Fatalf("boat %d still in the world", old)
	}
	for _, id := range []int32{me, riderID} {
		if x, y, _ := srv.PlayerPosition(t, id); x != reloadTown.X || y != reloadTown.Y {
			t.Fatalf("passenger %d at %d,%d, want ashore at %v", id, x, y, reloadTown)
		}
	}

	// The fare due went with the old boat. The new one, leaving Rune for
	// Primeval, starts its schedule at once, heard in town.
	sail(srv, 10)
	for _, id := range []int32{me, riderID} {
		if n := srv.PlayerItemCount(t, id, ticketID); n != 1 {
			t.Fatalf("passenger %d holds %d tickets after the reload, want 1", id, n)
		}
	}
	assertLog(t, "after the reload", reloadLog(t, srv, rider), say(1001), sound(fiveMinutes, newBoat.ObjectID(), runeDock))
	sail(srv, 300)
	if !newBoat.Moving() {
		t.Fatal("the reloaded boat did not leave on its schedule")
	}
}

// TestAdminReloadBoatWithoutBoats pins //reload boat on a server whose
// fleet AllowBoat left empty at boot: the routes read put out as on any
// reload.
func TestAdminReloadBoatWithoutBoats(t *testing.T) {
	t.Parallel()
	var fail atomic.Bool
	srv, gm, _ := bootReload(t, []route.BoatItinerary{reloadedItinerary()}, &fail)
	if n := len(srv.Boats.Boats()); n != 0 {
		t.Fatalf("fleet at boot = %d boats, want none", n)
	}

	gm.Send(encodeBuildCmd("reload boat"))
	log := reloadLog(t, srv, gm)
	boats := srv.Boats.Boats()
	if len(boats) != 1 {
		t.Fatalf("fleet after the reload = %d boats, want 1", len(boats))
	}
	assertLog(t, "reload", log, info(boats[0].ObjectID(), runeDock, 1000), reloadDone)
}

// TestAdminReloadBoatTwice pins two boat reloads in one command while a
// passenger other than the game master is still being put ashore: the
// fleet ends with one boat per itinerary read, not one per reload.
func TestAdminReloadBoatTwice(t *testing.T) {
	t.Parallel()
	var fail atomic.Bool
	srv, gm, _ := bootReload(t, []route.BoatItinerary{reloadedItinerary()}, &fail, passengerItinerary())
	old := srv.Boats.Boats()[0].ObjectID()
	rider, riderID := addRider(t, srv, "rider", "Rider", runeBoarding)
	board(t, srv, rider, old)

	gm.Send(encodeBuildCmd("reload boat boat"))
	reloadLog(t, srv, gm)
	reloadLog(t, srv, rider)
	if n := len(srv.Boats.Boats()); n != 1 {
		t.Fatalf("fleet after two reloads = %d boats, want 1", n)
	}
	if _, ok := srv.State.Object(old); ok {
		t.Fatalf("boat %d still in the world", old)
	}
	if x, y, _ := srv.PlayerPosition(t, riderID); x != reloadTown.X || y != reloadTown.Y {
		t.Fatalf("passenger at %d,%d, want ashore at %v", x, y, reloadTown)
	}
}

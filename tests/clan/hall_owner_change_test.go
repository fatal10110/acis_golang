package clan

import (
	"context"
	"database/sql"
	"encoding/binary"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: ClanHall.setOwner (ClanHall.java:237-292) closes every gate
// of the hall (Residence.closeDoors, Door.closeMe broadcasting the door's
// status) once the former owner's clan header is refreshed, then, with
// the new owner set, banishForeigners: ClanHallZone.banishForeigners
// (ClanHallZone.java:23-31) teleports every player in the hall's zone
// whose clan is not the owner to a random BANISH spawn of the hall, with
// a random offset of 20. ClanHall.free (ClanHall.java:195-233) closes the
// gates too. Moonstone Hall (22) lists the gates
// gludio_castle_agit_001_001 and gludio_castle_agit_001_002 and one BANISH
// spawn, (-15872, 123824, -3116).

const (
	rivalHallClanID = 0x70000013
	moonstoneGateA  = 24_220_001
	moonstoneGateB  = 24_220_002
)

var moonstoneBanish = location.Location{X: -15872, Y: 123824, Z: -3116}

// ownerChangeSetup is the world an owner change boots with: "Newbie" leads
// Hallkeepers, "Rivals" has no member online, and owner (one of the two
// clans) owns Moonstone Hall, its lease paid until paidUntil.
type ownerChangeSetup struct {
	owner     int
	paid      bool
	paidUntil int64
}

// moonstoneGate is an open door named after one of Moonstone Hall's gates,
// standing beside the spawn point so the player sees its status.
func moonstoneGate(id int, name string, x int) *door.Template {
	return &door.Template{
		ID:       id,
		Name:     name,
		Kind:     door.KindDoor,
		Level:    1,
		Position: location.Location{X: x, Y: 100, Z: gameservertest.SpawnZ},
		Coordinates: []location.Point{
			{X: x - 8, Y: 92}, {X: x + 8, Y: 92}, {X: x + 8, Y: 108}, {X: x - 8, Y: 108},
		},
		HP: 100, PDef: 10, MDef: 10, Height: 32,
		Opened:   true,
		OpenKind: door.OpenNPC,
	}
}

// bootOwnerChange boots Newbie inside Moonstone Hall's grounds, next to
// its two open gates, and drains the entry.
func bootOwnerChange(t *testing.T, s ownerChangeSetup) *gameservertest.Server {
	t.Helper()
	datapack.Require(t)
	halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
	if err != nil {
		t.Fatal(err)
	}
	decos, err := gamexml.LoadClanHallDeco(datapack.Path(t, "data", "xml", "clanHallDeco.xml"))
	if err != nil {
		t.Fatal(err)
	}
	form, err := zone.NewCuboid(-200, 200, -200, 200, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	set := commons.NewStatSet()
	set.Set("clanHallId", strconv.Itoa(moonstoneHall))
	grounds, err := zone.NewClanHall(1, form, set)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(grounds)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithResidences(halls, nil),
		gameservertest.WithDoors(
			moonstoneGate(moonstoneGateA, "gludio_castle_agit_001_001", -60),
			moonstoneGate(moonstoneGateB, "GLUDIO_CASTLE_AGIT_001_002", 80),
		),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedOwnerChange(t, db, s) }),
	)
	startInWorld(t, srv.Client)
	return srv
}

func seedOwnerChange(t *testing.T, db *sql.DB, s ownerChangeSetup) {
	t.Helper()
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatalf("seed owner change: %s: %v", q, err)
		}
	}
	exec("UPDATE characters SET clanid = ? WHERE char_name = 'Newbie'", hallClanID)
	exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
		SELECT ?, 'Hallkeepers', 4, obj_Id FROM characters WHERE char_name = 'Newbie'`, hallClanID)
	exec("INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (?, 'Rivals', 4)", rivalHallClanID)
	exec("INSERT INTO clanhall (id, ownerId, paid, paidUntil) VALUES (?, ?, ?, ?)", moonstoneHall, s.owner, s.paid, s.paidUntil)
}

// closedGates returns the door ids the DoorStatusUpdate frames among
// frames report closed, and the index of the last of them.
func closedGates(frames [][]byte) (ids []int32, last int) {
	last = -1
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeDoorStatusUpdate || len(f) < 21 {
			continue
		}
		if binary.LittleEndian.Uint32(f[5:]) == 1 {
			ids = append(ids, int32(binary.LittleEndian.Uint32(f[17:])))
			last = i
		}
	}
	return ids, last
}

// wantGatesClosed fails unless frames close both of Moonstone Hall's
// gates, and returns the index of the last close.
func wantGatesClosed(t *testing.T, frames [][]byte) int {
	t.Helper()
	ids, last := closedGates(frames)
	if len(ids) != 2 || ids[0] != moonstoneGateA || ids[1] != moonstoneGateB {
		t.Fatalf("gates closed = %v, want [%d %d] (opcodes % x)", ids, moonstoneGateA, moonstoneGateB, opcodes(frames))
	}
	return last
}

// TestClanHallOwnerChangeBanishesForeigners: the hall changing hands closes
// its gates after the former owner's clan header, then throws the former
// owner's member out of its grounds to the hall's banish point.
func TestClanHallOwnerChangeBanishesForeigners(t *testing.T) {
	t.Parallel()
	srv := bootOwnerChange(t, ownerChangeSetup{owner: hallClanID, paid: true, paidUntil: future()})
	rivals, ok := srv.Clans.Table().Get(rivalHallClanID)
	if !ok || !srv.Halls.SetOwner(moonstoneHall, rivals) {
		t.Fatal("give Moonstone Hall to Rivals")
	}
	frames := drainFrames(t, srv.Client)
	header := -1
	for i, f := range frames {
		if f[0] == serverpackets.OpcodePledgeShowInfoUpdate {
			header = i
			break
		}
	}
	lastClose := wantGatesClosed(t, frames)
	teleport := -1
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeTeleportToLocation {
			teleport = i
			break
		}
	}
	if header < 0 || teleport < 0 || !(header < lastClose && lastClose < teleport) {
		t.Fatalf("header at %d, last gate close at %d, teleport at %d; want them in that order (opcodes % x)", header, lastClose, teleport, opcodes(frames))
	}
	f := frames[teleport]
	x := int(int32(binary.LittleEndian.Uint32(f[5:])))
	y := int(int32(binary.LittleEndian.Uint32(f[9:])))
	z := int(int32(binary.LittleEndian.Uint32(f[13:])))
	if abs(x-moonstoneBanish.X) > 20 || abs(y-moonstoneBanish.Y) > 20 || z != moonstoneBanish.Z {
		t.Fatalf("teleported to (%d, %d, %d), want within 20 of %+v", x, y, z, moonstoneBanish)
	}
	if px, py, _ := srv.PlayerPosition(t, srv.SoleObjectID(t)); abs(px-moonstoneBanish.X) > 20 || abs(py-moonstoneBanish.Y) > 20 {
		t.Fatalf("player at (%d, %d), want at the banish point", px, py)
	}
}

// TestClanHallOwnerChangeKeepsNewOwnersMembers: a member of the clan that
// wins the hall stays in its grounds; the gates still close.
func TestClanHallOwnerChangeKeepsNewOwnersMembers(t *testing.T) {
	t.Parallel()
	srv := bootOwnerChange(t, ownerChangeSetup{owner: rivalHallClanID, paid: true, paidUntil: future()})
	keepers, ok := srv.Clans.Table().Get(hallClanID)
	if !ok || !srv.Halls.SetOwner(moonstoneHall, keepers) {
		t.Fatal("give Moonstone Hall to Hallkeepers")
	}
	frames := drainFrames(t, srv.Client)
	wantGatesClosed(t, frames)
	if _, ok := firstOpcode(frames, serverpackets.OpcodeTeleportToLocation); ok {
		t.Fatalf("the new owner's member was teleported (opcodes % x)", opcodes(frames))
	}
	if px, py, _ := srv.PlayerPosition(t, srv.SoleObjectID(t)); px != 10 || py != 20 {
		t.Fatalf("player at (%d, %d), want at its spawn point (10, 20)", px, py)
	}
}

// TestClanHallLostLeaseClosesGates: a hall lost to an unpaid lease closes
// its gates and throws no one out.
func TestClanHallLostLeaseClosesGates(t *testing.T) {
	t.Parallel()
	due := time.Now().Add(time.Hour).UnixMilli()
	srv := bootOwnerChange(t, ownerChangeSetup{owner: hallClanID, paid: false, paidUntil: due})
	if !srv.DrivesClock() {
		t.Skip("the lease is waited out on the test clock")
	}
	srv.Advance(t, time.Hour+time.Minute)
	frames := drainFrames(t, srv.Client)
	wantGatesClosed(t, frames)
	if _, ok := firstOpcode(frames, serverpackets.OpcodeTeleportToLocation); ok {
		t.Fatalf("a hall lost to its lease teleported its former owner (opcodes % x)", opcodes(frames))
	}
	if v, _ := srv.Halls.View(moonstoneHall); v.OwnerID != 0 {
		t.Fatalf("hall owner = %d, want free", v.OwnerID)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

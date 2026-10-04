package combat

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Shipped residences and access levels the restart selections read.
const (
	restartHallID   = 22 // a clan hall with OWNER spawns in clanHalls.xml
	restartCastleID = 6  // a castle with OWNER spawns in castles.xml

	restartClanID = 0x70000002

	userAccess    = 0 // isGM=false allowFixedRes=false
	generalGM     = 3 // isGM=false allowFixedRes=true
	adminAccess   = 7 // isGM=true  allowFixedRes=true
	restartToJail = 27

	restartToClanHallType = 1
	restartFixedType      = 4

	punishJailLevel = 2 // characters.punish_level of a jail term
)

// restartClan is the clan the dead character leads: the castle in
// clan_data.hasCastle and the hall its clanhall row names, 0 for none.
type restartClan struct{ castle, hall int }

// deathWindow is the Die packet's restart choices.
type deathWindow struct{ hall, castle, siegeHQ, fixed bool }

// restarter is how "Newbie" is stored before the login: the access level,
// the clan led (none when nil), and whether a jail term without end is
// served and the character stored dead (curHp 0).
type restarter struct {
	clan   *restartClan
	access int
	jailed bool
	dead   bool
}

// bootDeadAtRestart boots "Newbie" stored dead at the given access level,
// leading clan (no clan when nil), with the shipped clan halls, castles and
// access levels and the one-point restart table. It enters the world and
// returns the server, the object id, and the choices the login's Die
// packet offered.
func bootDeadAtRestart(t *testing.T, clan *restartClan, access int) (*gameservertest.Server, int32, deathWindow) {
	t.Helper()
	return bootRestarterDead(t, restarter{clan: clan, access: access, dead: true})
}

// bootRestarterDead boots r, which must be stored dead, enters the world
// and returns the server, the object id, and the Die window of the login.
func bootRestarterDead(t *testing.T, r restarter) (*gameservertest.Server, int32, deathWindow) {
	t.Helper()
	srv, objID := bootRestarter(t, r)
	c := srv.Client
	var window deathWindow
	for i := 0; ; i++ {
		frame := c.ReadWithTimeout(5 * time.Second)
		if frame == nil || i == 200 {
			t.Fatal("dead login sent no Die")
		}
		if frame[0] != serverpackets.OpcodeDie {
			continue
		}
		w := wireReader(frame[1:])
		if w.ReadInt32() != objID {
			continue
		}
		if town := w.ReadInt32(); town != 1 {
			t.Fatalf("Die to-village = %d, want 1", town)
		}
		window = deathWindow{hall: w.ReadInt32() == 1, castle: w.ReadInt32() == 1, siegeHQ: w.ReadInt32() == 1}
		w.ReadInt32() // sweepable
		window.fixed = w.ReadInt32() == 1
		break
	}
	drainUntilQuiet(t, c)
	if r.jailed {
		appearInJail(t, c)
	}
	return srv, objID, window
}

// bootRestarter boots "Newbie" stored as r with the shipped clan halls,
// castles and access levels and the one-point restart table, and sends
// the game start and EnterWorld; the login burst is left unread.
func bootRestarter(t *testing.T, r restarter) (*gameservertest.Server, int32) {
	t.Helper()
	datapack.Require(t)
	halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
	if err != nil {
		t.Fatalf("load clan halls: %v", err)
	}
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	adminData, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithRestartPoints(restartTable()),
		gameservertest.WithResidences(halls, castles),
		gameservertest.WithAdmin(adminData),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seedRestarter(t, db, r)
		}),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	c.Send(encodeEnterWorld())
	return srv, objID
}

// seedRestarter stores Newbie as r.
func seedRestarter(t *testing.T, db *sql.DB, r restarter) {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("seed restarter: %v", err)
		}
	}
	exec("UPDATE characters SET accesslevel = ? WHERE char_name = 'Newbie'", r.access)
	if r.dead {
		exec("UPDATE characters SET curHp = 0 WHERE char_name = 'Newbie'")
	}
	if r.jailed {
		exec("UPDATE characters SET punish_level = ?, punish_timer = 0 WHERE char_name = 'Newbie'", punishJailLevel)
	}
	clan := r.clan
	if clan == nil {
		return
	}
	exec("UPDATE characters SET clanid = ? WHERE char_name = 'Newbie'", restartClanID)
	exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id)
		SELECT ?, 'Lords', 5, ?, obj_Id FROM characters WHERE char_name = 'Newbie'`, restartClanID, clan.castle)
	if clan.hall != 0 {
		exec("INSERT INTO clanhall (id, ownerId, paid) VALUES (?, ?, 1)", clan.hall, restartClanID)
	}
}

// ownerSpawns is the OWNER spawn list a residence restart picks from.
func ownerSpawns(t *testing.T, kind string) []location.Location {
	t.Helper()
	var spawns map[residence.SpawnType][]location.Location
	switch kind {
	case "hall":
		halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
		if err != nil {
			t.Fatalf("load clan halls: %v", err)
		}
		hall, ok := halls.Get(restartHallID)
		if !ok {
			t.Fatalf("clan hall %d missing", restartHallID)
		}
		spawns = hall.Spawns
	case "castle":
		castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
		if err != nil {
			t.Fatalf("load castles: %v", err)
		}
		c, ok := castles.Get(restartCastleID)
		if !ok {
			t.Fatalf("castle %d missing", restartCastleID)
		}
		spawns = c.Spawns
	}
	owners := spawns[residence.SpawnOwner]
	if len(owners) == 0 {
		t.Fatalf("%s has no OWNER spawn", kind)
	}
	return owners
}

// readRestartTeleport reads until live's TeleportToLocation and returns its
// destination, failing when no Revive came before it.
func readRestartTeleport(t *testing.T, c *scriptedClient, objectID int32) location.Location {
	t.Helper()
	revived := false
	for i := 0; i < 50; i++ {
		frame := mustRead(t, c, "revive/teleport frame")
		switch frame[0] {
		case serverpackets.OpcodeRevive:
			revived = revived || wireReader(frame[1:]).ReadInt32() == objectID
		case serverpackets.OpcodeTeleportToLocation:
			r := wireReader(frame[1:])
			if r.ReadInt32() != objectID {
				continue
			}
			if !revived {
				t.Fatal("TeleportToLocation arrived before Revive")
			}
			return location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
		}
	}
	t.Fatal("TeleportToLocation never arrived after restart")
	return location.Location{}
}

// nearAny reports whether at lies within the restart scatter of one of
// points.
func nearAny(at location.Location, points []location.Location) bool {
	for _, p := range points {
		if abs(at.X-p.X) <= 25 && abs(at.Y-p.Y) <= 25 {
			return true
		}
	}
	return false
}

// TestRestartPointSelections pins RequestRestartPoint.portPlayer's per-type
// branches outside a siege, and the Die window offering them: the clan hall
// and castle restarts need the clan to own one and land on one of its OWNER
// spawns, the siege HQ restart finds no flag and lands in town, the fixed
// restart needs a GM (isGM, not allowFixedRes) and keeps the player where
// they fell, the jail restart needs the player jailed, and any other type
// is the town restart. A refused type sends nothing and leaves the player
// dead.
func TestRestartPointSelections(t *testing.T) {
	t.Parallel()
	hall := &restartClan{hall: restartHallID}
	lord := &restartClan{castle: restartCastleID}
	bare := &restartClan{}
	for _, tt := range []struct {
		name    string
		clan    *restartClan
		access  int
		request int32
		window  deathWindow
		dest    string // "hall", "castle", "town", "here" or "" (refused)
	}{
		{"hall owner to clan hall", hall, userAccess, 1, deathWindow{hall: true}, "hall"},
		{"hall owner to castle", hall, userAccess, 2, deathWindow{hall: true}, ""},
		{"castle owner to castle", lord, userAccess, 2, deathWindow{castle: true}, "castle"},
		{"castle owner to clan hall", lord, userAccess, 1, deathWindow{castle: true}, ""},
		{"no residence to clan hall", bare, userAccess, 1, deathWindow{}, ""},
		{"no residence to castle", bare, userAccess, 2, deathWindow{}, ""},
		{"no clan to clan hall", nil, userAccess, 1, deathWindow{}, ""},
		{"no clan to castle", nil, userAccess, 2, deathWindow{}, ""},
		{"siege HQ outside a siege", lord, userAccess, 3, deathWindow{castle: true}, "town"},
		{"fixed as a user", nil, userAccess, 4, deathWindow{}, ""},
		{"fixed with allowFixedRes but no GM", nil, generalGM, 4, deathWindow{fixed: true}, ""},
		{"fixed as a GM", nil, adminAccess, 4, deathWindow{fixed: true}, "here"},
		{"jail while free", nil, userAccess, restartToJail, deathWindow{}, ""},
		{"unknown type", hall, userAccess, 9, deathWindow{hall: true}, "town"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, objID, window := bootDeadAtRestart(t, tt.clan, tt.access)
			if window != tt.window {
				t.Fatalf("Die window = %+v, want %+v", window, tt.window)
			}
			c := srv.Client
			c.Send(encodeRequestRestartPoint(tt.request))
			if tt.dest == "" {
				for {
					frame := c.ReadWithTimeout(500 * time.Millisecond)
					if frame == nil {
						break
					}
					if frame[0] == serverpackets.OpcodeRevive || frame[0] == serverpackets.OpcodeTeleportToLocation {
						t.Fatalf("refused restart sent opcode %#x", frame[0])
					}
				}
				if !srv.PlayerDead(t, objID) {
					t.Fatal("refused restart revived the player")
				}
				return
			}
			var want []location.Location
			switch tt.dest {
			case "hall", "castle":
				want = ownerSpawns(t, tt.dest)
			case "town":
				want = []location.Location{townRestartPoint}
			case "here":
				want = []location.Location{playerOrigin}
			}
			if at := readRestartTeleport(t, c, objID); !nearAny(at, want) {
				t.Fatalf("restart destination = %+v, want within scatter of one of %+v", at, want)
			}
			if srv.PlayerDead(t, objID) {
				t.Fatal("player still dead after the restart")
			}
		})
	}
}

// jailPoint is where a jailed player is held.
var jailPoint = location.Location{X: -114356, Y: -249645, Z: -2984}

// appearInJail answers the login's jail teleport with Appearing, ending
// the teleport the restart and movement requests wait on.
func appearInJail(t *testing.T, c *scriptedClient) {
	t.Helper()
	c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	drainUntilQuiet(t, c)
}

// TestRestartPointJailOverride pins the jail override of
// RequestRestartPoint.portPlayer ahead of every per-type branch: a jailed
// player who dies restarts in the jail even when the type asked for would
// lead out of it. A jailed clan hall owner asking for the clan hall, and a
// jailed GM who walked away from the jail point asking for the fixed
// restart, both land within the scatter of the jail point.
func TestRestartPointJailOverride(t *testing.T) {
	t.Parallel()
	t.Run("hall owner to clan hall", func(t *testing.T) {
		t.Parallel()
		srv, objID, window := bootRestarterDead(t, restarter{
			clan: &restartClan{hall: restartHallID}, access: userAccess, jailed: true, dead: true,
		})
		if want := (deathWindow{hall: true}); window != want {
			t.Fatalf("Die window = %+v, want %+v", window, want)
		}
		c := srv.Client
		c.Send(encodeRequestRestartPoint(restartToClanHallType))
		if at := readRestartTeleport(t, c, objID); !nearAny(at, []location.Location{jailPoint}) {
			t.Fatalf("restart destination = %+v, want within scatter of the jail %+v", at, jailPoint)
		}
		if srv.PlayerDead(t, objID) {
			t.Fatal("player still dead after the restart")
		}
	})
	t.Run("GM to fixed", func(t *testing.T) {
		t.Parallel()
		srv, objID := bootRestarter(t, restarter{access: adminAccess, jailed: true})
		c := srv.Client
		drainUntilQuiet(t, c)
		appearInJail(t, c)
		// The login took the GM to the jail; walk off the jail point so
		// the fixed restart, which keeps the player where they fell,
		// cannot land there by itself.
		w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
		for _, v := range []int{jailPoint.X + 400, jailPoint.Y, jailPoint.Z, jailPoint.X, jailPoint.Y, jailPoint.Z, 1} {
			w.WriteInt32(int32(v))
		}
		c.Send(w.Bytes())
		drainUntilQuiet(t, c)
		srv.Advance(t, 8*time.Second)
		drainUntilQuiet(t, c)
		if x, y, _ := srv.PlayerPosition(t, objID); abs(x-jailPoint.X) < 200 && abs(y-jailPoint.Y) < 200 {
			t.Fatalf("GM at (%d,%d), want it walked at least 200 off the jail %+v", x, y, jailPoint)
		}
		srv.MarkPlayerDead(t, objID)
		c.Send(encodeRequestRestartPoint(restartFixedType))
		if at := readRestartTeleport(t, c, objID); !nearAny(at, []location.Location{jailPoint}) {
			t.Fatalf("restart destination = %+v, want within scatter of the jail %+v", at, jailPoint)
		}
		if srv.PlayerDead(t, objID) {
			t.Fatal("player still dead after the restart")
		}
	})
}

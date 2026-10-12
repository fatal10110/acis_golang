package combat

import (
	"context"
	"database/sql"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The siege restart world: castle restartCastleID is under siege over the
// dead player's position; siegeRivalClanID, led by an offline character,
// attacks it so the siege can start.
const (
	siegeRivalClanID   int32 = 0x70000012
	siegeRivalLeaderID int32 = 0x70000013

	// siegeRespawn is the attacker respawn delay the siege restart world
	// runs with.
	siegeRespawn = 3 * time.Second
)

// siegeRestarter is the dead player of the siege restart world.
type siegeRestarter struct {
	// side is the siege side of the player's clan: "OWNER" makes it the
	// castle's owner, "" registers it on no side. Ignored without a clan.
	side string
	// noClan leaves the player out of any clan.
	noClan bool
	// karma is the player's karma.
	karma int
	// dawnStrife puts the Seven Signs in seal validation with Dawn holding
	// the Seal of Strife; otherwise Dusk holds it.
	dawnStrife bool
	// x, when set, stores the player at that x coordinate instead of the
	// default one.
	x int
	// fieldEast, when set, is the battlefield's east edge instead of 1000.
	fieldEast int
}

// bootSiegeRestart boots r stored dead inside castle restartCastleID's
// battlefield with that castle's siege under way, enters the world and
// returns the server, the object id and the login's Die window. opts add to
// the world's boot options. An attacker or a player of no side entering
// the world on the battlefield is sent to the town restart point, which
// lies on the battlefield too; it lands there before this returns.
func bootSiegeRestart(t *testing.T, r siegeRestarter, opts ...gameservertest.Option) (*gameservertest.Server, int32, deathWindow) {
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
	east := 1_000
	if r.fieldEast != 0 {
		east = r.fieldEast
	}
	form, err := zone.NewCuboid(-1_000, east, -1_000, 1_000, -10_000, 10_000)
	if err != nil {
		t.Fatalf("battlefield: %v", err)
	}
	zones := zone.NewIndex()
	field := zone.NewSiege(1, form)
	field.ResidenceID = restartCastleID
	zones.Add(field)
	cfg := siege.DefaultConfig()
	cfg.AttackerRespawn = siegeRespawn
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithRestartPoints(restartTable()),
		gameservertest.WithResidences(halls, castles),
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(cfg),
		gameservertest.WithZones(zones),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedSiegeRestarter(t, db, r) }),
		gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) {
			ctx := context.Background()
			row, found, err := store.LoadStatus(ctx)
			if err != nil || !found {
				t.Fatalf("load seven signs status: found=%v err=%v", found, err)
			}
			row.Period = sevensigns.SealValidation
			row.SealOwners[2] = sevensigns.Dusk
			if r.dawnStrife {
				row.SealOwners[2] = sevensigns.Dawn
			}
			if err := store.SaveStatus(ctx, row); err != nil {
				t.Fatalf("seed seven signs status: %v", err)
			}
		}),
		// No period change fires during the test.
		gameservertest.WithSevenSignsTimer(func(time.Duration, func()) *time.Timer { return nil }),
	}, opts...)...)
	s, ok := srv.Sieges.Get(restartCastleID)
	if !ok {
		t.Fatalf("no siege of castle %d", restartCastleID)
	}
	// The siege starts before the player comes in, so its start does not
	// throw the player off the battlefield.
	s.Start()
	if !s.InProgress() {
		t.Fatal("siege did not start")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	c.Send(encodeEnterWorld())
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
		w.ReadInt32() // to village
		window = deathWindow{hall: w.ReadInt32() == 1, castle: w.ReadInt32() == 1, siegeHQ: w.ReadInt32() == 1}
		w.ReadInt32() // sweepable
		window.fixed = w.ReadInt32() == 1
		break
	}
	if indexOf(readQuiet(c), 0, serverpackets.OpcodeTeleportToLocation, objID) >= 0 {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
		drainUntilQuiet(t, c)
	}
	return srv, objID, window
}

// seedSiegeRestarter stores Newbie dead as r and the rival attacking clan.
func seedSiegeRestarter(t *testing.T, db *sql.DB, r siegeRestarter) {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("seed siege restarter: %v", err)
		}
	}
	exec("UPDATE characters SET curHp = 0, karma = ? WHERE char_name = 'Newbie'", r.karma)
	if r.x != 0 {
		exec("UPDATE characters SET x = ? WHERE char_name = 'Newbie'", r.x)
	}
	exec("INSERT INTO characters (account_name, obj_Id, char_name, level, clanid) VALUES ('rival', ?, 'Red', 40, ?)", siegeRivalLeaderID, siegeRivalClanID)
	exec("INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (?, 'Rivals', 5, ?)", siegeRivalClanID, siegeRivalLeaderID)
	exec("INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, 'ATTACKER')", restartCastleID, siegeRivalClanID)
	if r.noClan {
		return
	}
	castleID := 0
	if r.side == "OWNER" {
		castleID = restartCastleID
	}
	exec("UPDATE characters SET clanid = ? WHERE char_name = 'Newbie'", restartClanID)
	exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id)
		SELECT ?, 'Lords', 5, ?, obj_Id FROM characters WHERE char_name = 'Newbie'`, restartClanID, castleID)
	if r.side != "" && r.side != "OWNER" {
		exec("INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, ?)", restartCastleID, restartClanID, r.side)
	}
}

// castleSpawns is castle restartCastleID's spawn list of kind.
func castleSpawns(t *testing.T, kind residence.SpawnType) []location.Location {
	t.Helper()
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	c, ok := castles.Get(restartCastleID)
	if !ok {
		t.Fatalf("castle %d missing", restartCastleID)
	}
	if len(c.Spawns[kind]) == 0 {
		t.Fatalf("castle %d has no %v spawn", restartCastleID, kind)
	}
	return c.Spawns[kind]
}

// assertRestartRefused checks a refused restart sent neither Revive nor
// TeleportToLocation and left objID dead.
func assertRestartRefused(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	for _, frame := range srv.ReadQueued(t, srv.Client) {
		if frame[0] == serverpackets.OpcodeRevive || frame[0] == serverpackets.OpcodeTeleportToLocation {
			t.Fatalf("refused restart sent opcode %#x", frame[0])
		}
	}
	if !srv.PlayerDead(t, objID) {
		t.Fatal("refused restart revived the player")
	}
}

// TestSiegeRestartSides pins RequestRestartPoint.portPlayer and the Die
// window inside a siege in progress, Dusk holding the Seal of Strife: the
// castle restart takes an owner or defender to one of the besieged castle's
// OWNER spawns, which the Die window offers; an attacker's castle restart
// lands in town once its respawn delay passed; a clan registered on no
// side, a clan waiting for approval and a clanless player may not use it.
// No clan holds a siege headquarters, so the siege HQ restart lands in town.
func TestSiegeRestartSides(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		r       siegeRestarter
		request int32
		window  deathWindow
		dest    string // "castle", "town" or "" (refused)
	}{
		{"owner to castle", siegeRestarter{side: "OWNER"}, 2, deathWindow{castle: true}, "castle"},
		{"defender to castle", siegeRestarter{side: "DEFENDER"}, 2, deathWindow{castle: true}, "castle"},
		{"defender to siege HQ", siegeRestarter{side: "DEFENDER"}, 3, deathWindow{castle: true}, "town"},
		{"attacker to castle", siegeRestarter{side: "ATTACKER"}, 2, deathWindow{}, "town"},
		{"attacker to siege HQ", siegeRestarter{side: "ATTACKER"}, 3, deathWindow{}, "town"},
		{"unregistered clan to castle", siegeRestarter{}, 2, deathWindow{}, ""},
		{"pending clan to castle", siegeRestarter{side: "PENDING"}, 2, deathWindow{}, ""},
		{"no clan to castle", siegeRestarter{noClan: true}, 2, deathWindow{}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, objID, window := bootSiegeRestart(t, tt.r)
			if window != tt.window {
				t.Fatalf("Die window = %+v, want %+v", window, tt.window)
			}
			c := srv.Client
			c.Send(encodeRequestRestartPoint(tt.request))
			if tt.r.side == "ATTACKER" {
				srv.Advance(t, siegeRespawn)
			}
			if tt.dest == "" {
				assertRestartRefused(t, srv, objID)
				return
			}
			want := []location.Location{townRestartPoint}
			if tt.dest == "castle" {
				want = castleSpawns(t, residence.SpawnOwner)
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

// TestSiegeAttackerRestartWaits pins the attacker respawn delay of
// RequestRestartPoint.runImpl: a dead attacker inside the siege stays dead
// with nothing sent until the delay has passed, then restarts to town. A
// second request made while it waits restarts it no second time.
func TestSiegeAttackerRestartWaits(t *testing.T) {
	t.Parallel()
	srv, objID, _ := bootSiegeRestart(t, siegeRestarter{side: "ATTACKER"})
	c := srv.Client
	c.Send(encodeRequestRestartPoint(0))
	srv.Advance(t, siegeRespawn/2)
	c.Send(encodeRequestRestartPoint(0))
	srv.Advance(t, siegeRespawn/2-100*time.Millisecond)
	assertRestartRefused(t, srv, objID)

	srv.Advance(t, 100*time.Millisecond)
	if at := readRestartTeleport(t, c, objID); !nearAny(at, []location.Location{townRestartPoint}) {
		t.Fatalf("restart destination = %+v, want within scatter of %+v", at, townRestartPoint)
	}
	if srv.PlayerDead(t, objID) {
		t.Fatal("player still dead after the delayed restart")
	}
	srv.Advance(t, siegeRespawn)
	for _, frame := range srv.ReadQueued(t, c) {
		if frame[0] == serverpackets.OpcodeTeleportToLocation {
			t.Fatal("the second request teleported the player again")
		}
	}
}

// TestSiegeRestartSealOfStrife pins the town restart inside a siege in
// progress (RestartPointData.getLocationToTeleport): while Dawn holds the
// Seal of Strife in seal validation, a restart to town lands at one of the
// besieged castle's OTHER spawns, or CHAOTIC ones with karma; while Dusk
// holds it, at the nearest town.
func TestSiegeRestartSealOfStrife(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		r    siegeRestarter
		want func(t *testing.T) []location.Location
	}{
		{"dawn", siegeRestarter{side: "DEFENDER", dawnStrife: true}, func(t *testing.T) []location.Location {
			return castleSpawns(t, residence.SpawnOther)
		}},
		{"dawn with karma", siegeRestarter{side: "DEFENDER", karma: 100, dawnStrife: true}, func(t *testing.T) []location.Location {
			return castleSpawns(t, residence.SpawnChaotic)
		}},
		{"dusk", siegeRestarter{side: "DEFENDER"}, func(*testing.T) []location.Location {
			return []location.Location{townRestartPoint}
		}},
		{"dusk with karma", siegeRestarter{side: "DEFENDER", karma: 100}, func(*testing.T) []location.Location {
			return []location.Location{chaosRestartPoint}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, objID, _ := bootSiegeRestart(t, tt.r)
			c := srv.Client
			c.Send(encodeRequestRestartPoint(0))
			want := tt.want(t)
			if at := readRestartTeleport(t, c, objID); !nearAny(at, want) {
				t.Fatalf("restart destination = %+v, want within scatter of one of %+v", at, want)
			}
		})
	}
}

package combat

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: RequestRestartPoint type 3 restarts at
// RestartPointData.getLocationToTeleport(SIEGE_FLAG): the clan's flag when
// the clan attacks the castle siege in progress at the player's position
// (Siege.getFlag checks the ATTACKER side), else the clan's flag when a
// clan hall siege is fought there (ClanHallSiege.getFlag checks no side),
// else the town. The Die window offers it while the clan holds a flag and,
// with a castle siege there, its side is ATTACKER, or, with none there, it
// attacks the clan hall siege fought there (ClanHallSiege.checkSide).

// siegeHallID is Devastated Castle, the siegable hall whose siege the hall
// restart world fights.
const siegeHallID int32 = 34

// siegeHQPoint is where the restarter's clan's headquarters stands, on the
// battlefield away from the town restart point and the player.
var siegeHQPoint = location.Location{X: -600, Y: -500, Z: -300}

// seedRestartHallSiege stores Devastated Castle's lease, so its siege can
// be bound, the rival clan attacking it, and the restarter's clan too when
// r attacks.
func seedRestartHallSiege(t *testing.T, db *sql.DB, r siegeRestarter) {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("seed hall siege: %v", err)
		}
	}
	exec("INSERT INTO clanhall (id, endDate) VALUES (?, ?)", siegeHallID, time.Now().Add(7*24*time.Hour).UnixMilli())
	exec("INSERT INTO clanhall_siege_attackers VALUES (?, ?)", siegeHallID, siegeRivalClanID)
	if r.side == "ATTACKER" && !r.noClan {
		exec("INSERT INTO clanhall_siege_attackers VALUES (?, ?)", siegeHallID, restartClanID)
	}
}

// startRestartHallSiege starts Devastated Castle's siege now.
func startRestartHallSiege(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	if !srv.DrivesClock() {
		t.Skip("starting the hall siege needs the driven clock")
	}
	s, ok := srv.Halls.Siege(siegeHallID)
	if !ok || !s.Instant() {
		t.Fatal("no Devastated Castle siege to start")
	}
	srv.Advance(t, 0)
	if !s.InProgress() {
		t.Fatal("Devastated Castle siege did not start")
	}
}

// putUpRestartHQ stands a siege headquarters of the restarter's clan at
// siegeHQPoint.
func putUpRestartHQ(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	cl, ok := srv.Clans.Table().Get(restartClanID)
	if !ok {
		t.Fatalf("clan %#x not loaded", restartClanID)
	}
	flag := srv.SpawnFolkNPCAt(t, npc.SiegeFlagTemplate(cl.Name(), false), siegeHQPoint)
	if !flag.StandFor(cl) {
		t.Fatal("the clan already holds a headquarters")
	}
}

// TestSiegeHQRestart pins the siege HQ restart and the Die window's siege HQ
// choice against a castle siege and a clan hall siege in progress over the
// dead player. Only a castle siege's attacker waits out the respawn delay
// (RequestRestartPoint.runImpl reads only CastleManager.getActiveSiege): a
// hall siege's attacker restarts at once, with the clock left alone.
func TestSiegeHQRestart(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		r       siegeRestarter
		request int32
		window  deathWindow
		hq      bool // the restart lands at the headquarters, else in town
	}{
		{"castle attacker with HQ", siegeRestarter{side: "ATTACKER", hq: true}, 3, deathWindow{siegeHQ: true}, true},
		{"castle attacker with HQ to castle", siegeRestarter{side: "ATTACKER", hq: true}, 2, deathWindow{siegeHQ: true}, false},
		{"castle defender with HQ", siegeRestarter{side: "DEFENDER", hq: true}, 3, deathWindow{castle: true}, false},
		{"castle unregistered with HQ", siegeRestarter{hq: true}, 3, deathWindow{}, false},
		{"hall attacker with HQ", siegeRestarter{hall: true, side: "ATTACKER", hq: true}, 3, deathWindow{siegeHQ: true}, true},
		{"hall attacker without HQ", siegeRestarter{hall: true, side: "ATTACKER"}, 3, deathWindow{}, false},
		{"hall unregistered with HQ", siegeRestarter{hall: true, hq: true}, 3, deathWindow{}, true},
		{"hall no clan", siegeRestarter{hall: true, noClan: true}, 3, deathWindow{}, false},
		{"off the hall battlefield with HQ", siegeRestarter{hall: true, side: "ATTACKER", hq: true, x: 3000}, 3, deathWindow{}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, objID, window := bootSiegeRestart(t, tt.r)
			if window != tt.window {
				t.Fatalf("Die window = %+v, want %+v", window, tt.window)
			}
			c := srv.Client
			c.Send(encodeRequestRestartPoint(tt.request))
			if tt.r.side == "ATTACKER" && !tt.r.hall {
				// Only a castle siege's attacker waits out the respawn delay.
				srv.Advance(t, siegeRespawn)
			}
			want := townRestartPoint
			if tt.hq {
				want = siegeHQPoint
			}
			if at := readRestartTeleport(t, c, objID); !nearAny(at, []location.Location{want}) {
				t.Fatalf("restart destination = %+v, want within scatter of %+v", at, want)
			}
			if srv.PlayerDead(t, objID) {
				t.Fatal("player still dead after the restart")
			}
		})
	}
}

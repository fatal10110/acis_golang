package combat

import (
	"context"
	"database/sql"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The death the restarter comes back from: level 5 (deathLossTable's
// 1000-3000 band) at 1300 exp, down from 1500.
const (
	hallExpAfterDeath  = 1300
	hallExpBeforeDeath = 1500
)

// bootHallExpRestarter boots "Newbie" stored dead after the death above,
// leading a clan that owns castle castleID (0 for none) and clan hall
// restartHallID, which rents the restore-exp function at restoreLevel (0
// for none). It enters the world past the login's Die.
func bootHallExpRestarter(t *testing.T, castleID, restoreLevel int) (*gameservertest.Server, int32) {
	t.Helper()
	datapack.Require(t)
	halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
	if err != nil {
		t.Fatalf("load clan halls: %v", err)
	}
	decos, err := gamexml.LoadClanHallDeco(datapack.Path(t, "data", "xml", "clanHallDeco.xml"))
	if err != nil {
		t.Fatalf("load clan hall decorations: %v", err)
	}
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithLevels(deathLossTable(t)),
		gameservertest.WithRestartPoints(restartTable()),
		gameservertest.WithResidences(halls, castles),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			ctx := context.Background()
			exec := func(q string, args ...any) {
				if _, err := db.ExecContext(ctx, q, args...); err != nil {
					t.Fatalf("seed hall exp restarter: %v", err)
				}
			}
			exec("UPDATE characters SET curHp = 0, exp = ?, expBeforeDeath = ?, clanid = ? WHERE char_name = 'Newbie'",
				hallExpAfterDeath, hallExpBeforeDeath, restartClanID)
			exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id)
				SELECT ?, 'Lords', 5, ?, obj_Id FROM characters WHERE char_name = 'Newbie'`, restartClanID, castleID)
			farFuture := time.Now().Add(30 * 24 * time.Hour).UnixMilli()
			exec("INSERT INTO clanhall (id, ownerId, paid, paidUntil) VALUES (?, ?, 1, ?)", restartHallID, restartClanID, farFuture)
			if restoreLevel > 0 {
				exec("INSERT INTO clanhall_functions (hall_id, type, lvl, lease, rate, endTime) VALUES (?, ?, ?, ?, ?, ?)",
					restartHallID, hallmodel.FuncRestoreExp, restoreLevel, 0, (24 * time.Hour).Milliseconds(), farFuture)
			}
		}),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	c.Send(encodeEnterWorld())
	drainUntilQuiet(t, c)
	if !srv.PlayerDead(t, objID) {
		t.Fatal("restarter logged in alive")
	}
	return srv, objID
}

// TestRestartToClanHallRestoresExp pins RequestRestartPoint.portPlayer's
// clan hall branch (:80-89): a type-1 restart by the owner of a hall that
// rents the restore-exp function at level N gives back N% of the
// experience the death took (Player.restoreExp), and the death snapshot is
// spent; without the function nothing comes back. No other restart type
// restores experience, even for a clan whose hall rents it.
func TestRestartToClanHallRestoresExp(t *testing.T) {
	t.Parallel()
	hall := ownerSpawns(t, "hall")
	for _, tt := range []struct {
		name         string
		castleID     int
		restoreLevel int
		request      int32
		dest         []location.Location
		wantExp      int64
	}{
		// round((1500 - 1300) * 20 / 100) = 40.
		{"clan hall renting restore-exp 20", 0, 20, restartToClanHallType, hall, hallExpAfterDeath + 40},
		{"clan hall renting none", 0, 0, restartToClanHallType, hall, hallExpAfterDeath},
		{"castle while the hall rents restore-exp", restartCastleID, 20, 2, ownerSpawns(t, "castle"), hallExpAfterDeath},
		{"town while the hall rents restore-exp", 0, 20, 0, []location.Location{townRestartPoint}, hallExpAfterDeath},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, objID := bootHallExpRestarter(t, tt.castleID, tt.restoreLevel)
			c := srv.Client
			c.Send(encodeRequestRestartPoint(tt.request))
			if at := readRestartTeleport(t, c, objID); !nearAny(at, tt.dest) {
				t.Fatalf("restart destination = %+v, want within scatter of one of %+v", at, tt.dest)
			}
			if srv.PlayerDead(t, objID) {
				t.Fatal("player still dead after the restart")
			}
			c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
			drainUntilQuiet(t, c)
			exp, before := persistedExp(t, srv, c, objID)
			if exp != tt.wantExp {
				t.Fatalf("exp after the restart = %d, want %d", exp, tt.wantExp)
			}
			if restored := tt.wantExp != hallExpAfterDeath; restored && before != 0 {
				t.Fatalf("expBeforeDeath after the restore = %d, want 0", before)
			}
		})
	}
}

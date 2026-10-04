package skills

import (
	"context"
	"database/sql"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Shipped residences a residence recall lands in, and the clan owning them.
const (
	recallHallID   = 22 // Moonstone Hall, with OWNER spawns in clanHalls.xml
	recallCastleID = 6  // a castle with OWNER spawns in castles.xml
	recallClanID   = 0x70000003
)

// shippedResidences loads the shipped clan halls and castles.
func shippedResidences(t *testing.T) (*hallmodel.Table, *castledata.Table) {
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
	return halls, castles
}

// residenceOwnerClan seeds Newbie leading a clan that owns castle castleID
// (0 for none) and clan hall hallID (0 for none).
func residenceOwnerClan(t *testing.T, castleID, hallID int) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		ctx := context.Background()
		exec := func(q string, args ...any) {
			if _, err := db.ExecContext(ctx, q, args...); err != nil {
				t.Fatalf("seed residence owner clan: %v", err)
			}
		}
		exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id)
			SELECT ?, 'Lords', 5, ?, obj_Id FROM characters WHERE char_name = 'Newbie'`, recallClanID, castleID)
		exec("UPDATE characters SET clanid = ? WHERE char_name = 'Newbie'", recallClanID)
		if hallID != 0 {
			exec("INSERT INTO clanhall (id, ownerId, paid) VALUES (?, ?, 1)", hallID, recallClanID)
		}
	})
}

// nearAnyOf reports whether x, y lie within the recall scatter of one of
// points.
func nearAnyOf(x, y int, points []location.Location) bool {
	for _, p := range points {
		if near(x, y, p) {
			return true
		}
	}
	return false
}

// TestResidenceRecallLandsAtOwnerSpawn casts the shipped Blessed Scroll of
// Escape: Clan Hall (2177) and Castle (2178) skills for a clan owning the
// residence. L2SkillTeleport.useSkill resolves them through
// RestartPointData.getLocationToTeleport(CLAN_HALL / CASTLE): the owning
// clan's member lands within 20 of one of the residence's OWNER spawns
// (Residence.getRndSpawn(OWNER), teleportTo(loc, 20)), not in town.
func TestResidenceRecallLandsAtOwnerSpawn(t *testing.T) {
	t.Parallel()
	halls, castles := shippedResidences(t)
	hall, ok := halls.Get(recallHallID)
	if !ok {
		t.Fatalf("clan hall %d missing", recallHallID)
	}
	castle, ok := castles.Get(recallCastleID)
	if !ok {
		t.Fatalf("castle %d missing", recallCastleID)
	}
	town := location.Location{X: -5000, Y: 2000, Z: 30}
	for _, tt := range []struct {
		name             string
		skill            modelskill.ID
		castleID, hallID int
		spawns           []location.Location
	}{
		{"clan hall", 2177, 0, recallHallID, hall.Spawns[residence.SpawnOwner]},
		{"castle", 2178, recallCastleID, 0, castle.Spawns[residence.SpawnOwner]},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if len(tt.spawns) == 0 {
				t.Fatal("residence has no OWNER spawn")
			}
			def := shippedSkill(t, tt.skill, 1)
			srv, c, objID := bootRecallCaster(t, def,
				gameservertest.WithRestartPoints(recallTown(town)),
				gameservertest.WithResidences(halls, castles),
				residenceOwnerClan(t, tt.castleID, tt.hallID),
			)
			// A clan leader's login burst carries the clan packets the
			// plain burst reader does not expect; only the world entry
			// matters here.
			c.Send(encodeRequestGameStart(0))
			c.Send(encodeEnterWorld())
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
			srv.AdvanceUntil(t, "residence recall", func() bool {
				x, y, _ := srv.PlayerPosition(t, objID)
				return nearAnyOf(x, y, tt.spawns)
			})
		})
	}
}

package skills

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: with no castle siege fought where the caster stands,
// L2SkillSiegeFlag.check reads the clan hall siege whose battlefield is on
// there (ClanHallManager.getActiveSiege) and requires the caster's clan to
// attack it (checkSide ATTACKER); the rest of the check is the castle's.
// ClanHallSiege.endSiege takes every attacker's flag out of the world
// (clan.setFlag(null)).

// devastatedCastleID is the siegable hall 34.
const devastatedCastleID int32 = 34

// bootHallHQ boots Devastated Castle's siege under way with its battlefield
// and HQ ground over the caster, the caster's clan registered on it when
// attacks is set.
func bootHallHQ(t *testing.T, attacks bool) *hqWorld {
	t.Helper()
	datapack.Require(t)
	halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	field := zone.NewSiege(1, gateBox(t, true))
	field.ResidenceID = int(devastatedCastleID)
	zones.Add(field)
	zones.Add(zone.NewHQ(2, gateBox(t, true)))
	defs := hqSkills(t)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Sieger", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
		gameservertest.WithClanHalls(halls, nil),
		gameservertest.WithResidences(halls, nil),
		gameservertest.WithHallSiegeScript(devastatedCastleID, clanhall.SiegeScript{}),
		gameservertest.WithZones(zones),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			type stmt struct {
				q    string
				args []any
			}
			stmts := []stmt{
				{"UPDATE characters SET clanid = ?, maxMp = 1000, curMp = 500 WHERE char_name = 'Sieger'", []any{hqClanID}},
				{"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) SELECT ?, 'Siegers', 5, obj_Id FROM characters WHERE char_name = 'Sieger'", []any{hqClanID}},
				{"INSERT INTO clanhall (id, endDate) VALUES (?, ?)", []any{devastatedCastleID, time.Now().Add(7 * 24 * time.Hour).UnixMilli()}},
				// Rivals attacks either way, so the siege goes ahead.
				{"INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (?, 'Rivals', 5)", []any{hqRivalClanID}},
				{"INSERT INTO clanhall_siege_attackers VALUES (?, ?)", []any{devastatedCastleID, hqRivalClanID}},
			}
			if attacks {
				stmts = append(stmts, stmt{"INSERT INTO clanhall_siege_attackers VALUES (?, ?)", []any{devastatedCastleID, hqClanID}})
			}
			for _, s := range stmts {
				if _, err := db.ExecContext(context.Background(), s.q, s.args...); err != nil {
					t.Fatalf("seed hall siege clan: %s: %v", s.q, err)
				}
			}
		}),
	)
	if !srv.DrivesClock() {
		t.Skip("the flag cast's hit time needs the driven clock")
	}
	objID := srv.SoleObjectID(t)
	for _, def := range defs {
		seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	}
	srv.GiveItem(t, objID, hqCostItemID, hqCostHeld)
	enterBesieged(t, srv.Client)
	s, ok := srv.Halls.Siege(devastatedCastleID)
	if !ok || !s.Instant() {
		t.Fatal("no Devastated Castle siege to start")
	}
	srv.Advance(t, 0)
	drainUntilQuiet(t, srv.Client)
	if !field.Active() {
		t.Fatal("Devastated Castle's battlefield is off")
	}
	w := &hqWorld{srv: srv, c: srv.Client, objID: objID}
	w.clan = hqClan(t, srv, hqClanID)
	return w
}

// TestHallSiegeFlagCast lets an attacking clan's leader put up a
// headquarters on a hall siege's battlefield; the siege's end takes it out
// of the world. A leader of a clan not attacking the hall siege is refused
// S1_CANNOT_BE_USED naming Build Headquarters.
func TestHallSiegeFlagCast(t *testing.T) {
	t.Parallel()
	t.Run("attacker", func(t *testing.T) {
		t.Parallel()
		w := bootHallHQ(t, true)
		w.putUp(t, buildHQSkillID)
		if len(w.flags()) != 1 {
			t.Fatalf("headquarters in the world = %d, want 1", len(w.flags()))
		}
		if _, ok := w.clan.Headquarters(); !ok {
			t.Fatal("the clan holds no headquarters")
		}
		s, _ := w.srv.Halls.Siege(devastatedCastleID)
		s.End()
		w.srv.Settle(t)
		if len(w.flags()) != 0 {
			t.Fatal("the headquarters stands after the hall siege's end")
		}
		if _, ok := w.clan.Headquarters(); ok {
			t.Fatal("the clan holds its headquarters after the hall siege's end")
		}
	})
	t.Run("not attacking", func(t *testing.T) {
		t.Parallel()
		w := bootHallHQ(t, false)
		w.c.Send(encodeRequestMagicSkillUse(buildHQSkillID, false, false))
		frames := collectUntilQuiet(t, w.c)
		if !slices.Equal(opcodes(frames), []byte{serverpackets.OpcodeSystemMessage}) {
			t.Fatalf("refused flag cast answered %x, want one SystemMessage", opcodes(frames))
		}
		assertSystemMessageSkillFrame(t, frames[0], serverpackets.SystemMessageS1CannotBeUsed, buildHQSkillID, 1)
	})
}

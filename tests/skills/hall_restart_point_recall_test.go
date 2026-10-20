package skills

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: a town recall (Scroll of Escape, 2013) goes where a town
// restart does (RestartPointData.getLocationToTeleport): on the battlefield
// of a flag war hall siege, to the restart point the siege keeps for the
// caster's clan (FlagWar.getClanRestartPoint, RestartPointData.java
// 323-329); a clan with none goes to town.

// TestHallSiegeClanRestartPointRecall casts the shipped Scroll of Escape
// on Devastated Castle's battlefield, its siege under way and keeping a
// restart point for the caster's clan or for another clan only.
func TestHallSiegeClanRestartPointRecall(t *testing.T) {
	t.Parallel()
	datapack.Require(t)
	halls, err := gamexml.LoadClanHalls(datapack.Path(t, "data", "xml", "clanHalls.xml"))
	if err != nil {
		t.Fatal(err)
	}
	town := location.Location{X: -5000, Y: 2000, Z: 30}
	point := location.Location{X: 3000, Y: -3000, Z: 30}
	for _, tt := range []struct {
		name   string
		clanID int32
		want   location.Location
	}{
		{"clan with a restart point", recallClanID, point},
		{"clan without one", recallRivalClanID, town},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			form, err := zone.NewCuboid(-1_000, 1_000, -1_000, 1_000, -10_000, 10_000)
			if err != nil {
				t.Fatalf("battlefield: %v", err)
			}
			zones := zone.NewIndex()
			field := zone.NewSiege(1, form)
			field.ResidenceID = int(devastatedCastleID)
			zones.Add(field)
			def := shippedSkill(t, 2013, 1)
			srv, c, objID := bootRecallCaster(t, def,
				gameservertest.WithRestartPoints(recallTown(town)),
				gameservertest.WithClanHalls(halls, nil),
				gameservertest.WithResidences(halls, nil),
				gameservertest.WithHallSiegeScript(devastatedCastleID, clanhall.SiegeScript{}),
				gameservertest.WithZones(zones),
				gameservertest.WithClanSeed(func(db *sql.DB) {
					ctx := context.Background()
					for _, s := range []struct {
						q    string
						args []any
					}{
						{`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
							SELECT ?, 'Lords', 5, obj_Id FROM characters WHERE char_name = 'Newbie'`, []any{recallClanID}},
						{"UPDATE characters SET clanid = ? WHERE char_name = 'Newbie'", []any{recallClanID}},
						{"INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (?, 'Rivals', 5)", []any{recallRivalClanID}},
						{"INSERT INTO clanhall (id, endDate) VALUES (?, ?)", []any{devastatedCastleID, time.Now().Add(7 * 24 * time.Hour).UnixMilli()}},
						{"INSERT INTO clanhall_siege_attackers VALUES (?, ?)", []any{devastatedCastleID, recallClanID}},
					} {
						if _, err := db.ExecContext(ctx, s.q, s.args...); err != nil {
							t.Fatalf("seed hall siege: %v", err)
						}
					}
				}),
			)
			skipOnWallClock(t, srv)
			s, ok := srv.Halls.Siege(devastatedCastleID)
			if !ok || !s.Instant() {
				t.Fatal("no Devastated Castle siege to start")
			}
			// The siege starts before the caster comes in, so its start does
			// not throw the caster off the battlefield.
			srv.Advance(t, 0)
			if !s.InProgress() || !field.Active() {
				t.Fatal("Devastated Castle's siege did not start")
			}
			s.SetClanRestartPoint(tt.clanID, point)
			c.Send(encodeRequestGameStart(0))
			c.Send(encodeEnterWorld())
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
			drainUntilQuiet(t, c)
			if !srv.PlayerCastingNow(t, objID) {
				t.Fatal("2013 did not start a cast")
			}
			srv.Advance(t, 19*time.Second)
			srv.AdvanceUntil(t, "hall siege recall", func() bool {
				x, y, _ := srv.PlayerPosition(t, objID)
				return near(x, y, tt.want)
			})
		})
	}
}

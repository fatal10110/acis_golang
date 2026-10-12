package skills

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The rival clan attacking castle recallCastleID in the siege recall world,
// led by an offline character.
const (
	recallRivalClanID   int32 = 0x70000014
	recallRivalLeaderID int32 = 0x70000015
)

// TestSiegeCastleRecallFollowsSide casts the shipped Scroll of Escape:
// Castle (2041, no castle-owner condition, unlike 2178) inside castle
// recallCastleID's siege in progress, for a clan owning no castle.
// RestartPointData.getLocationToTeleport(CASTLE) takes a defender to one of
// the besieged castle's OWNER spawns; an attacker owns and defends nothing,
// so it goes to town.
func TestSiegeCastleRecallFollowsSide(t *testing.T) {
	t.Parallel()
	halls, castles := shippedResidences(t)
	castle, ok := castles.Get(recallCastleID)
	if !ok {
		t.Fatalf("castle %d missing", recallCastleID)
	}
	town := location.Location{X: -5000, Y: 2000, Z: 30}
	for _, tt := range []struct {
		side string
		want []location.Location
	}{
		{"DEFENDER", castle.Spawns[residence.SpawnOwner]},
		{"ATTACKER", []location.Location{town}},
	} {
		t.Run(tt.side, func(t *testing.T) {
			t.Parallel()
			if len(tt.want) == 0 {
				t.Fatal("castle has no OWNER spawn")
			}
			form, err := zone.NewCuboid(-1_000, 1_000, -1_000, 1_000, -10_000, 10_000)
			if err != nil {
				t.Fatalf("battlefield: %v", err)
			}
			zones := zone.NewIndex()
			field := zone.NewSiege(1, form)
			field.ResidenceID = recallCastleID
			zones.Add(field)
			def := shippedSkill(t, 2041, 1)
			srv, c, objID := bootRecallCaster(t, def,
				gameservertest.WithRestartPoints(recallTown(town)),
				gameservertest.WithResidences(halls, castles),
				gameservertest.WithCastles(castles),
				gameservertest.WithSieges(siege.DefaultConfig()),
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
						{"INSERT INTO characters (account_name, obj_Id, char_name, level, clanid) VALUES ('rival', ?, 'Red', 40, ?)", []any{recallRivalLeaderID, recallRivalClanID}},
						{"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (?, 'Rivals', 5, ?)", []any{recallRivalClanID, recallRivalLeaderID}},
						{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, 'ATTACKER')", []any{recallCastleID, recallRivalClanID}},
						{"INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (?, ?, ?)", []any{recallCastleID, recallClanID, tt.side}},
					} {
						if _, err := db.ExecContext(ctx, s.q, s.args...); err != nil {
							t.Fatalf("seed siege clans: %v", err)
						}
					}
				}),
			)
			s, ok := srv.Sieges.Get(recallCastleID)
			if !ok {
				t.Fatalf("no siege of castle %d", recallCastleID)
			}
			// The siege starts before the caster comes in, so its start
			// does not throw the caster off the battlefield.
			s.Start()
			if !s.InProgress() {
				t.Fatal("siege did not start")
			}
			c.Send(encodeRequestGameStart(0))
			c.Send(encodeEnterWorld())
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
			drainUntilQuiet(t, c)
			if !srv.PlayerCastingNow(t, objID) {
				t.Fatal("2041 did not start a cast")
			}
			// The cast hits after 20 s.
			srv.Advance(t, 19*time.Second)
			srv.AdvanceUntil(t, "siege castle recall", func() bool {
				x, y, _ := srv.PlayerPosition(t, objID)
				return nearAnyOf(x, y, tt.want)
			})
		})
	}
}

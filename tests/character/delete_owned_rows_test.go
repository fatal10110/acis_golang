package character

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The ids sit at the bottom of the production allocator's range, so a
// rebuilt allocator would hand the victim's id out first were it free.
const (
	ownedVictimID = int32(idfactory.FirstObjectID)
	ownedSpouseID = ownedVictimID + 1
	ownedGhostID  = ownedVictimID + 2
	ownedCoupleID = ownedVictimID + 3
)

// TestDeletePurgesOwnedRowsAndRetiresID deletes a married character that
// has a bookmark and a buffer scheme (#4676): its rows in bookmarks,
// buffer_schemes and mods_wedding go with it, from the database and from the
// couples and schemes the shutdown save would write back, whichever side of
// the marriage it was on, while its spouse keeps its own rows. A restart
// then removes the rows of a character that no longer exists and never
// hands the deleted character's id to a new object.
func TestDeletePurgesOwnedRowsAndRetiresID(t *testing.T) {
	t.Parallel()
	for _, side := range []struct {
		name               string
		requester, partner int32
	}{
		{"requester", ownedVictimID, ownedSpouseID},
		{"partner", ownedSpouseID, ownedVictimID},
	} {
		t.Run(side.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			schemes := schemebuffer.New(schemebuffer.DefaultConfig(), nil, nil)
			schemes.Restore([]schemebuffer.Row{
				{OwnerID: ownedVictimID, Name: "Mine"},
				{OwnerID: ownedSpouseID, Name: "Theirs"},
			}, zerolog.Nop())
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacterDeleteAfter(0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSchemeBuffer(schemes),
				gameservertest.WithSeed(func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
					for _, c := range []struct {
						id            int32
						account, name string
					}{{ownedVictimID, "player1", "Victim"}, {ownedSpouseID, "player2", "Spouse"}} {
						ch, err := player.NewCharacter(c.id, gameservertest.ClassTemplate(), c.account, c.name, 1, 0, 0, player.SexMale)
						if err != nil {
							t.Fatalf("seed %s: %v", c.name, err)
						}
						if err := chars.Create(ctx, ch); err != nil {
							t.Fatalf("store %s: %v", c.name, err)
						}
					}
					db := sqltest.SharedDB(t)
					for _, id := range []int32{ownedVictimID, ownedSpouseID} {
						seedOwnedRows(t, db, id)
					}
					execOwned(t, db, "INSERT INTO mods_wedding (id, requesterId, partnerId) VALUES (?, ?, ?)", ownedCoupleID, side.requester, side.partner)
				}),
			)
			if srv.Couples.CoupleID(ownedVictimID) != ownedCoupleID {
				t.Fatalf("couple of the victim at boot = %d, want %d", srv.Couples.CoupleID(ownedVictimID), ownedCoupleID)
			}

			c := srv.Client
			c.Send(encodeRequestCharacterDelete(0))
			if reply := c.Read(); reply[0] != serverpackets.OpcodeCharDeleteOk {
				t.Fatalf("opcode = %#x, want CharDeleteOk (%#x)", reply[0], serverpackets.OpcodeCharDeleteOk)
			}
			c.Read() // CharSelectInfo refresh

			db := srv.DB
			for _, table := range []string{"bookmarks", "buffer_schemes"} {
				if got := ownedRowOwners(t, db, table); !slices.Equal(got, []int32{ownedSpouseID}) {
					t.Fatalf("%s owners after the delete = %v, want only the spouse %d", table, got, ownedSpouseID)
				}
			}
			if n := countOwned(t, db, "SELECT COUNT(*) FROM mods_wedding"); n != 0 {
				t.Fatalf("mods_wedding rows after the delete = %d, want 0", n)
			}
			if id := srv.Couples.CoupleID(ownedSpouseID); id != 0 {
				t.Fatalf("spouse's couple after the delete = %d, want none", id)
			}
			srv.SaveCouples(t)
			if n := countOwned(t, db, "SELECT COUNT(*) FROM mods_wedding"); n != 0 {
				t.Fatalf("mods_wedding rows after the shutdown save = %d, want 0", n)
			}
			for _, row := range schemes.Rows() {
				if row.OwnerID == ownedVictimID {
					t.Fatalf("schemes the shutdown save writes = %+v, still holding the deleted character's", schemes.Rows())
				}
			}

			// Rows a deletion left behind before it removed them all, a
			// marriage to a missing character among them.
			seedOwnedRows(t, db, ownedGhostID)
			execOwned(t, db, "INSERT INTO mods_wedding (id, requesterId, partnerId) VALUES (?, ?, ?)", ownedCoupleID, ownedSpouseID, ownedGhostID)
			execOwned(t, db, "INSERT INTO mods_wedding (id, requesterId, partnerId) VALUES (?, ?, ?)", ownedCoupleID+1, ownedGhostID, ownedSpouseID)

			ids, err := idfactory.New(ctx, db, zerolog.Nop())
			if err != nil {
				t.Fatalf("restart allocator: %v", err)
			}
			for _, table := range []string{"bookmarks", "buffer_schemes"} {
				if got := ownedRowOwners(t, db, table); !slices.Equal(got, []int32{ownedSpouseID}) {
					t.Fatalf("%s owners after the restart = %v, want only the spouse %d", table, got, ownedSpouseID)
				}
			}
			if n := countOwned(t, db, "SELECT COUNT(*) FROM mods_wedding"); n != 0 {
				t.Fatalf("mods_wedding rows after the restart = %d, want 0", n)
			}
			id, err := ids.NextID()
			if err != nil {
				t.Fatalf("first id after the restart: %v", err)
			}
			if id == ownedVictimID {
				t.Fatalf("first id after the restart = %d, the deleted character's", id)
			}
		})
	}
}

func execOwned(t *testing.T, db *sql.DB, stmt string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, stmt, args...); err != nil {
		t.Fatalf("%s: %v", stmt, err)
	}
}

// seedOwnedRows stores a bookmark and a buffer scheme of owner.
func seedOwnedRows(t *testing.T, db *sql.DB, owner int32) {
	t.Helper()
	execOwned(t, db, "INSERT INTO bookmarks (name, obj_Id, x, y, z) VALUES ('Home', ?, 1, 2, 3)", owner)
	execOwned(t, db, "INSERT INTO buffer_schemes (object_id, scheme_name, skills) VALUES (?, 'Mine', '')", owner)
}

func countOwned(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// ownedRowOwners returns the owner ids table stores rows of, in order.
func ownedRowOwners(t *testing.T, db *sql.DB, table string) []int32 {
	t.Helper()
	column := map[string]string{"bookmarks": "obj_Id", "buffer_schemes": "object_id"}[table]
	rows, err := db.QueryContext(context.Background(), "SELECT DISTINCT "+column+" FROM "+table+" ORDER BY "+column)
	if err != nil {
		t.Fatalf("read %s: %v", table, err)
	}
	defer rows.Close()
	var out []int32
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("read %s: %v", table, err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read %s: %v", table, err)
	}
	return out
}

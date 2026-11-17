package character

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/dbtest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// savedRow is what a logout writes of the characters row and of its two
// subclass rows.
type savedRow struct {
	x, y, z, heading, online int
	sp, lastAccess           int64
	subSP                    [2]int64
	subLevel                 [2]int
}

func readSavedRow(t *testing.T, srv *gameservertest.Server, objID int32) savedRow {
	t.Helper()
	ctx := context.Background()
	var r savedRow
	if err := srv.DB.QueryRowContext(ctx, "SELECT x, y, z, heading, online, sp, lastAccess FROM characters WHERE obj_Id = ?", objID).
		Scan(&r.x, &r.y, &r.z, &r.heading, &r.online, &r.sp, &r.lastAccess); err != nil {
		t.Fatalf("read characters row: %v", err)
	}
	for i := range 2 {
		if err := srv.DB.QueryRowContext(ctx, "SELECT sp, level FROM character_subclasses WHERE char_obj_id = ? AND class_index = ?", objID, i+1).
			Scan(&r.subSP[i], &r.subLevel[i]); err != nil {
			t.Fatalf("read subclass %d row: %v", i+1, err)
		}
	}
	return r
}

// TestLogoutSaveIsAllOrNothing pins that the save a logout writes - the
// characters row with its position, progression and offline mark, and the
// rows of its subclasses - lands whole or not at all (#4673): a failure on
// the second subclass row leaves every row as it was before the logout.
func TestLogoutSaveIsAllOrNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		fail bool
	}{
		{name: "saved", fail: false},
		{name: "second subclass row fails", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var subTemplates []*player.Template
			for _, id := range []int{1, 4} {
				tmpl := gameservertest.ClassTemplate()
				tmpl.ID = id
				subTemplates = append(subTemplates, tmpl)
			}
			fault := &dbtest.Fault{}
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Saver", 40, 500),
				gameservertest.WithWantChars(1),
				gameservertest.WithClassTemplates(subTemplates...),
				gameservertest.WithDBFault(fault),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			ctx := context.Background()
			for i, class := range []int{1, 4} {
				if _, err := srv.DB.ExecContext(ctx, `INSERT INTO character_subclasses (char_obj_id, class_id, exp, sp, level, class_index) VALUES (?, ?, 0, 10, 40, ?)`, objID, class, i+1); err != nil {
					t.Fatalf("seed subclass %d: %v", i+1, err)
				}
			}

			c.Send(encodeRequestGameStart(0))
			c.Read() // SSQInfo
			c.Read() // CharSelected
			c.Send(encodeEnterWorld())
			readEnterWorldBurst(t, c)

			// The rows read apart from the live character from here on, so
			// the save is told from the rows it would replace.
			for _, q := range []string{
				"UPDATE characters SET x = 1, y = 2, z = 3, heading = 4, sp = 5, lastAccess = 6 WHERE obj_Id = ?",
				"UPDATE character_subclasses SET sp = 7, level = 8 WHERE char_obj_id = ?",
			} {
				if _, err := srv.DB.ExecContext(ctx, q, objID); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}
			before := readSavedRow(t, srv, objID)
			if tc.fail {
				fault.FailNth("UPDATE character_subclasses", 2)
			}

			c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
			if reply := c.Read(); reply[0] != serverpackets.OpcodeLeaveWorld {
				t.Fatalf("logout opcode = %#x, want LeaveWorld (%#x)", reply[0], serverpackets.OpcodeLeaveWorld)
			}
			c.ExpectClosed()

			after := readSavedRow(t, srv, objID)
			if tc.fail {
				if after != before {
					t.Fatalf("rows after a failed save = %+v, want them untouched: %+v", after, before)
				}
				return
			}
			if after.x == 1 || after.y == 2 || after.z == 3 || after.sp != 500 || after.online != 0 || after.lastAccess == 6 {
				t.Fatalf("characters row after the save = %+v, want the live position, sp 500, offline and a new lastAccess", after)
			}
			if after.subSP != [2]int64{10, 10} || after.subLevel != [2]int{40, 40} {
				t.Fatalf("subclass rows after the save = sp %v level %v, want sp 10 and level 40 each", after.subSP, after.subLevel)
			}
		})
	}
}

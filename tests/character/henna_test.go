package character

import (
	"context"
	"database/sql"
	"encoding/binary"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func TestEnterWorldRestoresHennaInfoAndBonuses(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Dyer", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHennaSeed(func(db *sql.DB, hennas *gamesql.HennaStore) {
			var objID int32
			if err := db.QueryRowContext(context.Background(),
				`SELECT obj_Id FROM characters WHERE account_name = ?`, "player1").Scan(&objID); err != nil {
				t.Fatalf("lookup character: %v", err)
			}
			if _, err := db.ExecContext(context.Background(),
				`UPDATE characters SET classid = 1 WHERE obj_Id = ?`, objID); err != nil {
				t.Fatalf("set classid: %v", err)
			}
			if err := hennas.Insert(context.Background(), objID, 1, 1); err != nil {
				t.Fatalf("insert henna: %v", err)
			}
		}),
	)
	objID := srv.SoleObjectID(t)

	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	for {
		frame := c.Read()
		if frame[0] == serverpackets.OpcodeCharSelected {
			break
		}
		if frame[0] != serverpackets.OpcodeSSQInfo {
			t.Fatalf("selection frame opcode = %#x, want SSQInfo/CharSelected", frame[0])
		}
	}
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c)
	hennaFrame := frames[2]
	// INT=0 STR=1 CON=-3→253 MEN=0 DEX=0 WIT=0, maxSlots=2, count=1, symbol 1 active
	want := []byte{serverpackets.OpcodeHennaInfo, 0, 1, 253, 0, 0, 0}
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 1)
	if string(hennaFrame) != string(want) {
		t.Fatalf("HennaInfo = %x, want %x", hennaFrame, want)
	}

	live, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("live player missing")
	}
	snapper, ok := live.(interface{ HennaSnapshot() henna.Snapshot })
	if !ok {
		t.Fatalf("live player %T missing HennaSnapshot", live)
	}
	snap := snapper.HennaSnapshot()
	if snap.STR != 1 || snap.CON != -3 || len(snap.Equipped) != 1 {
		t.Fatalf("live snapshot = %+v", snap)
	}
}

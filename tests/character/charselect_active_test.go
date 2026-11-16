package character

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/dbtest"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// charSelectSlot is one slot of a CharSelectInfo: its name and whether the
// client highlights it.
type charSelectSlot struct {
	name   string
	active bool
}

// decodeCharSelectInfo reads the slots of a CharSelectInfo frame.
func decodeCharSelectInfo(t *testing.T, frame []byte) []charSelectSlot {
	t.Helper()
	if frame[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want CharSelectInfo", frame[0])
	}
	r := wire.NewReader(frame[1:])
	slots := make([]charSelectSlot, r.ReadInt32())
	for i := range slots {
		slots[i].name = r.ReadString()
		r.ReadInt32() // character id
		r.ReadString()
		// Session id through class id: 71 four-byte fields.
		for range 71 {
			r.ReadInt32()
		}
		slots[i].active = r.ReadInt32() == 1
		r.ReadUint8()
		r.ReadInt32()
	}
	if err := r.Err(); err != nil {
		t.Fatalf("decode CharSelectInfo: %v", err)
	}
	if r.Remaining() != 0 {
		t.Fatalf("CharSelectInfo has %d trailing bytes", r.Remaining())
	}
	return slots
}

// readCharSelectSlots reads frames until a CharSelectInfo and decodes it.
func readCharSelectSlots(t *testing.T, c *testsupport.ScriptedClient, step string) []charSelectSlot {
	t.Helper()
	for range 200 {
		frame := c.Read()
		if frame == nil {
			t.Fatalf("%s: CharSelectInfo never arrived", step)
		}
		if frame[0] == serverpackets.OpcodeCharSelectInfo {
			return decodeCharSelectInfo(t, frame)
		}
	}
	t.Fatalf("%s: no CharSelectInfo within 200 frames", step)
	return nil
}

// assertActive fails unless exactly the slot named want is highlighted.
func assertActive(t *testing.T, step string, slots []charSelectSlot, want string) {
	t.Helper()
	for _, s := range slots {
		if s.active != (s.name == want) {
			t.Fatalf("%s: slots = %+v, want only %q highlighted", step, slots, want)
		}
	}
}

// lastPlayed is the character of account the list highlights by default:
// the first slot (lowest object id) holding the latest lastAccess.
func lastPlayed(t *testing.T, srv *gameservertest.Server, account string) string {
	t.Helper()
	var name string
	if err := srv.DB.QueryRowContext(context.Background(),
		"SELECT char_name FROM characters WHERE account_name = ? AND lastAccess > 0 ORDER BY lastAccess DESC, obj_Id ASC LIMIT 1",
		account).Scan(&name); err != nil {
		t.Fatalf("read last played character: %v", err)
	}
	return name
}

// TestCharSelectInfoHighlightedSlot pins which slot each character list
// highlights (CharSelectInfo.java): a delete or a restore answers with the
// first slot, while the login, a creation and a restart answer with the slot
// played most recently. The account's second slot is played more recently
// than its first, so the two rules disagree.
func TestCharSelectInfoHighlightedSlot(t *testing.T) {
	t.Parallel()
	const account = "slotorder"
	srv := gameservertest.Boot(t, gameservertest.WithReuseDelays(0, 0))
	first := srv.SeedCharacterFor(t, account, "Older", 1, 0)
	second := srv.SeedCharacterFor(t, account, "Newer", 1, 0)
	if first.ID >= second.ID {
		t.Fatalf("seeded ids %d, %d out of slot order", first.ID, second.ID)
	}
	for id, at := range map[int32]int64{first.ID: 1000, second.ID: 2000} {
		if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET lastAccess = ? WHERE obj_Id = ?", at, id); err != nil {
			t.Fatalf("set lastAccess: %v", err)
		}
	}
	key := link.SessionKey{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4}
	srv.IssueLoginSession(account, key)
	c := srv.DialAuthLogin(t, account, key)

	assertActive(t, "login", readCharSelectSlots(t, c, "login"), "Newer")

	c.Send(encodeRequestCharacterCreate("Created", 0, 0, 0, 1, 0, 0))
	assertActive(t, "create", readCharSelectSlots(t, c, "create"), lastPlayed(t, srv, account))

	c.Send(encodeRequestCharacterDelete(1))
	if frame := c.Read(); frame[0] != serverpackets.OpcodeCharDeleteOk {
		t.Fatalf("delete reply = %#x, want CharDeleteOk", frame[0])
	}
	assertActive(t, "delete", readCharSelectSlots(t, c, "delete"), "Older")

	c.Send(encodeCharacterRestore(1))
	assertActive(t, "restore", readCharSelectSlots(t, c, "restore"), "Older")

	c.Send(encodeRequestGameStart(1))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeEnterWorld())
	readUntil(t, c, serverpackets.OpcodeActionFailed)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntil(t, c, serverpackets.OpcodeRestartResponse)
	slots := readCharSelectSlots(t, c, "restart")
	if want := lastPlayed(t, srv, account); want != "Newer" {
		t.Fatalf("last played after restart = %q, want Newer", want)
	}
	assertActive(t, "restart", slots, "Newer")
}

// TestDeleteDatabaseFaultSendsNoFailure pins a delete the database refuses
// (GameClient.markToDeleteChar's catch, RequestCharacterDelete's -1 case):
// no CharDeleteFail, only the refreshed list, which the fault leaves empty.
// The purge path (DeleteCharAfterDays 0) answers the same way.
func TestDeleteDatabaseFaultSendsNoFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		after time.Duration
	}{
		{"scheduled", 7 * 24 * time.Hour},
		{"purge", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fault := &dbtest.Fault{}
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Doomed", 1, 0), gameservertest.WithWantChars(1),
				gameservertest.WithReuseDelays(0, 0),
				gameservertest.WithCharacterDeleteAfter(tc.after),
				gameservertest.WithDBFault(fault),
			)
			id := srv.SoleObjectID(t)
			fault.Arm()
			srv.Client.Send(encodeRequestCharacterDelete(0))
			frame := srv.Client.Read()
			if frame[0] != serverpackets.OpcodeCharSelectInfo {
				t.Fatalf("delete reply = %#x, want CharSelectInfo only", frame[0])
			}
			if slots := decodeCharSelectInfo(t, frame); len(slots) != 0 {
				t.Fatalf("list under the fault = %+v, want empty", slots)
			}
			fault.Disarm()
			var deleteAt int64
			if err := srv.DB.QueryRowContext(context.Background(),
				"SELECT COALESCE(deletetime,0) FROM characters WHERE obj_Id = ?", id).Scan(&deleteAt); err != nil {
				t.Fatalf("read character row: %v", err)
			}
			if deleteAt != 0 {
				t.Fatalf("deletetime = %d, want 0", deleteAt)
			}
		})
	}
}

package character

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/dbtest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// tableRows counts every row of table.
func tableRows(t *testing.T, srv *gameservertest.Server, table string) int {
	t.Helper()
	var n int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	return n
}

// TestCreateFailingStarterItemLeavesNoRows pins that a creation failing on
// its second starter item writes nothing (#4675): the client hears
// CREATION_FAILED, no characters, items or character_shortcuts row is left
// behind, and the next creation lists only the character it made.
func TestCreateFailingStarterItemLeavesNoRows(t *testing.T) {
	t.Parallel()
	starter := gameservertest.ClassTemplate()
	starter.Items = []player.StarterItem{{ItemID: int(item.AdenaID), Count: 100}, {ItemID: 20, Count: 1}, {ItemID: 1463, Count: 50}}
	fault := &dbtest.Fault{}
	srv := gameservertest.Boot(t, gameservertest.WithWantChars(0), gameservertest.WithClassTemplates(starter), gameservertest.WithDBFault(fault))
	c := srv.Client
	tables := []string{"characters", "items", "character_shortcuts"}
	before := make(map[string]int, len(tables))
	for _, table := range tables {
		before[table] = tableRows(t, srv, table)
	}

	fault.FailNth("INSERT INTO items", 2)
	c.Send(encodeRequestCharacterCreate("Halfmade", 0, 0, 0, 0, 0, 0))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeCharCreateFail {
		t.Fatalf("reply = %#x, want CharCreateFail", reply[0])
	}
	if got := int32(reply[1]) | int32(reply[2])<<8 | int32(reply[3])<<16 | int32(reply[4])<<24; got != int32(serverpackets.CharCreateFailReasonCreationFailed) {
		t.Fatalf("CharCreateFail reason = %d, want creation failed", got)
	}
	for _, table := range tables {
		if got := tableRows(t, srv, table); got != before[table] {
			t.Fatalf("%s rows after the failed creation = %d, want %d", table, got, before[table])
		}
	}

	// The retry of the same name succeeds and the list it answers with holds
	// that one character only.
	c.Send(encodeRequestCharacterCreate("Halfmade", 0, 0, 0, 0, 0, 0))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharCreateOk {
		t.Fatalf("retry reply = %#x, want CharCreateOk", reply[0])
	}
	list := c.Read()
	if list[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("frame after CharCreateOk = %#x, want CharSelectInfo", list[0])
	}
	if n := int32(list[1]) | int32(list[2])<<8 | int32(list[3])<<16 | int32(list[4])<<24; n != 1 {
		t.Fatalf("CharSelectInfo lists %d characters, want 1", n)
	}
	if got := tableRows(t, srv, "items"); got != before["items"]+len(starter.Items) {
		t.Fatalf("items rows after the retry = %d, want %d: every starter item of the new character", got, before["items"]+len(starter.Items))
	}
}

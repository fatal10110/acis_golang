package lifecycle

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/dbtest"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestDBOutageCharacterListGoesOutEmpty logs an account in while the game
// server's database is down. Nothing about the outage reaches the client as
// an error: the character list goes out empty and the connection stays at
// character select. With the list empty, the slot the character held is
// refused as an unknown slot is, with no answer at all, and a creation the
// database cannot store fails like any refused creation. Once the database
// is back, the next list the client asks for is the real one and the
// character enters the world.
func TestDBOutageCharacterListGoesOutEmpty(t *testing.T) {
	t.Parallel()
	fault := &dbtest.Fault{}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithCapturedLog(),
		gameservertest.WithDBFault(fault),
	)
	srv.SeedCharacterFor(t, "outage", "Outaged", 1, 0)

	fault.Arm()
	c := srv.DialClient(t, "outage", 0)
	if !strings.Contains(srv.LogText(), "list characters") {
		t.Fatal("no log line for the character list the database could not read")
	}

	c.Send(encodeRequestGameStart(0))
	c.ExpectNoFrame()

	c.Send(encodeOutageCharacterCreate("Another"))
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeCharCreateFail, "create with the database down")
	if got := int32(binary.LittleEndian.Uint32(reply[1:5])); got != int32(serverpackets.CharCreateFailReasonCreationFailed) {
		t.Fatalf("CharCreateFail reason = %d, want %d (creation failed)", got, serverpackets.CharCreateFailReasonCreationFailed)
	}
	c.ExpectNoFrame()

	fault.Disarm()
	c.Send(encodeOutageCharacterRestore(0))
	assertCharSelectCount(t, c.Read(), 1)
	startInWorld(t, c)
}

// TestDBOutageGameStartReturnsTheEmptyList selects a character from a list
// read while the database was up, with the database gone before the
// selection reads the character back. The selection fails without a word
// about the outage: no character is selected or enters the world, nothing
// is written, and the client is answered with the character list, empty
// because the database is still down. Once the database is back the
// refreshed list holds the character again and the same slot enters.
func TestDBOutageGameStartReturnsTheEmptyList(t *testing.T) {
	t.Parallel()
	fault := &dbtest.Fault{}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithDBFault(fault),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	before := outageCharacterRow(t, srv.DB, objID)
	beforeItems := outageItemRows(t, srv.DB, objID)

	fault.Arm()
	c.Send(encodeRequestGameStart(0))
	assertCharSelectCount(t, c.Read(), 0)
	c.ExpectNoFrame()
	if _, inWorld := srv.State.Player(objID); inWorld {
		t.Fatal("character entered the world from a selection the database could not load")
	}
	fault.Disarm()

	if after := outageCharacterRow(t, srv.DB, objID); !slices.Equal(after, before) {
		t.Fatalf("character row changed by the failed selection:\nbefore %q\nafter  %q", before, after)
	}
	if after := outageItemRows(t, srv.DB, objID); after != beforeItems {
		t.Fatalf("item rows changed by the failed selection: %d, want %d", after, beforeItems)
	}

	c.Send(encodeOutageCharacterRestore(0))
	assertCharSelectCount(t, c.Read(), 1)
	startInWorld(t, c)
	if _, inWorld := srv.State.Player(objID); !inWorld {
		t.Fatal("character missing from the world after the database came back")
	}
}

// TestDBOutageMidSessionSavesFailSilently takes the database away while two
// players are in the world. Every save the server makes from then on fails
// and is only logged: the autosave sweep sends neither player anything and
// leaves both in the world, and a logout still takes its player out of the
// world and closes the connection. Nothing the failed saves carried reaches
// the rows, so the relog after the database returns restores the state last
// saved, not a mix of it and the lost one. The persistence worker survives
// the failures: the next save, for the other player, lands once the database
// is back. A shutdown with the database down again still finishes, with the
// final item flush reporting the failure instead of hanging on it.
func TestDBOutageMidSessionSavesFailSilently(t *testing.T) {
	t.Parallel()
	fault := &dbtest.Fault{}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithDBFault(fault),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	bystander := srv.SeedCharacterFor(t, "bystander", "Bystander", 1, 0)
	bc := srv.DialClient(t, "bystander", 1)
	startInWorld(t, c)
	startInWorld(t, bc)
	drainUntilQuiet(t, c)

	// A cycle with the database up lands the damage taken so far: the rows
	// hold the state the session last saved.
	srv.DamagePlayerHP(t, objID, 10)
	autosaveCycle(t, srv)
	savedHP := srv.PlayerCurrentHP(t, objID)
	if got := outageCurHP(t, srv.DB, objID); got != savedHP {
		t.Fatalf("precondition failed: row HP %d after an autosave cycle, want %d", got, savedHP)
	}
	saved := outageCharacterRow(t, srv.DB, objID)

	srv.DamagePlayerHP(t, objID, 10)
	lostHP := srv.PlayerCurrentHP(t, objID)
	if lostHP >= savedHP {
		t.Fatalf("precondition failed: HP %d after damage, want below %d", lostHP, savedHP)
	}
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, bc)

	fault.Arm()
	autosaveCycle(t, srv)
	c.ExpectNoFrame()
	bc.ExpectNoFrame()
	if got := srv.PlayerCurrentHP(t, objID); got != lostHP {
		t.Fatalf("HP after the failed autosave = %d, want %d", got, lostHP)
	}
	if _, inWorld := srv.State.Player(bystander.ObjectID()); !inWorld {
		t.Fatal("bystander left the world on the failed autosave")
	}
	bc.Send(encodeRequestItemList())
	assertFrameOpcode(t, bc.Read(), serverpackets.OpcodeItemList, "bystander item list during the outage")
	drainUntilQuiet(t, bc)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	readUntilOpcode(t, c, serverpackets.OpcodeLeaveWorld)
	c.ExpectClosed()
	srv.FlushPersistence(t)
	if _, inWorld := srv.State.Player(objID); inWorld {
		t.Fatal("player still in the world after logging out with the database down")
	}
	if got := outageCharacterRow(t, srv.DB, objID); !slices.Equal(got, saved) {
		t.Fatalf("character row changed while the database was down:\nsaved %q\nnow   %q", saved, got)
	}

	fault.Disarm()
	srv.DamagePlayerHP(t, bystander.ObjectID(), 5)
	drainUntilQuiet(t, bc)
	autosaveCycle(t, srv)
	if got, want := outageCurHP(t, srv.DB, bystander.ObjectID()), srv.PlayerCurrentHP(t, bystander.ObjectID()); got != want {
		t.Fatalf("bystander row HP = %d after the database came back, want %d: the persistence worker stopped saving", got, want)
	}

	rc := srv.DialClient(t, srv.Account(), 1)
	startInWorld(t, rc)
	if got := srv.PlayerCurrentHP(t, objID); got != savedHP {
		t.Fatalf("relog HP = %d, want the saved %d (the lost save held %d)", got, savedHP, lostHP)
	}

	fault.Arm()
	srv.Stop()
	srv.FlushPersistence(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.ItemInstances.Save(ctx); err != nil && !errors.Is(err, dbtest.ErrFault) {
		t.Fatalf("final item flush with the database down = %v, want nil or the fault", err)
	}
	if ctx.Err() != nil {
		t.Fatal("final item flush with the database down ran out its budget")
	}
}

// autosaveCycle moves the autosave clock through one full save interval, so
// every tracked player is saved exactly once.
func autosaveCycle(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	for range task.AutosaveInterval / task.AutosaveInitialDelay {
		srv.TickAutosave(t)
	}
}

// assertCharSelectCount checks frame is a CharSelectInfo listing want
// characters.
func assertCharSelectCount(t *testing.T, frame []byte, want int) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeCharSelectInfo, "character list")
	if got := int(binary.LittleEndian.Uint32(frame[1:5])); got != want {
		t.Fatalf("CharSelectInfo lists %d characters, want %d", got, want)
	}
}

// outageCharacterRow returns objID's characters row, every column as text.
func outageCharacterRow(t *testing.T, db *sql.DB, objID int32) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), "SELECT * FROM characters WHERE obj_Id = ?", objID)
	if err != nil {
		t.Fatalf("read character row: %v", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatalf("no characters row for %d", objID)
	}
	raw := make([]sql.NullString, len(cols))
	dest := make([]any, len(cols))
	for i := range raw {
		dest[i] = &raw[i]
	}
	if err := rows.Scan(dest...); err != nil {
		t.Fatalf("scan character row: %v", err)
	}
	out := make([]string, len(cols))
	for i, v := range raw {
		out[i] = cols[i] + "=" + v.String
	}
	return out
}

// outageCurHP returns objID's stored current HP.
func outageCurHP(t *testing.T, db *sql.DB, objID int32) int {
	t.Helper()
	var hp int
	if err := db.QueryRowContext(context.Background(), "SELECT curHp FROM characters WHERE obj_Id = ?", objID).Scan(&hp); err != nil {
		t.Fatalf("read curHp: %v", err)
	}
	return hp
}

// outageItemRows returns how many item rows objID owns.
func outageItemRows(t *testing.T, db *sql.DB, objID int32) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM items WHERE owner_id = ?", objID).Scan(&n); err != nil {
		t.Fatalf("count item rows: %v", err)
	}
	return n
}

func encodeOutageCharacterRestore(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeCharacterRestore)
	w.WriteInt32(slot)
	return w.Bytes()
}

// encodeOutageCharacterCreate asks for a human fighter named name.
func encodeOutageCharacterCreate(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterCreate)
	w.WriteString(name)
	for range 3 { // race, sex, class: human male fighter
		w.WriteInt32(0)
	}
	for range 6 { // the unused base stats
		w.WriteInt32(0)
	}
	for range 3 { // hair style, hair color, face
		w.WriteInt32(0)
	}
	return w.Bytes()
}

package admin

import (
	"context"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// encodeCreateCharacter builds a RequestCharacterCreate for a human
// fighter named name.
func encodeCreateCharacter(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterCreate)
	w.WriteString(name)
	for range 12 { // race, sex, class, six unused, hair style, hair color, face
		w.WriteInt32(0)
	}
	return w.Bytes()
}

// nameRows counts the characters named name, matched case-insensitively.
func nameRows(t *testing.T, srv *gameservertest.Server, name string) int {
	t.Helper()
	var n int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM characters WHERE LOWER(char_name) = LOWER(?)", name).Scan(&n); err != nil {
		t.Fatalf("count %s rows: %v", name, err)
	}
	return n
}

// assertNameCreatable asserts a new client creates a character named
// name: a refused rename has given its hold on the name back.
func assertNameCreatable(t *testing.T, srv *gameservertest.Server, name string) {
	t.Helper()
	creator := srv.DialClient(t, "creator", 0)
	creator.Send(encodeCreateCharacter(name))
	if reply := creator.Read(); reply[0] != serverpackets.OpcodeCharCreateOk {
		t.Fatalf("creation of %s reply = %x, want CharCreateOk", name, reply)
	}
	if n := nameRows(t, srv, name); n != 1 {
		t.Fatalf("%d rows hold %s, want 1", n, name)
	}
}

// nameHold is a hold a test releases once it has acted in it. A test
// also frees it at cleanup, registered after the boot so it runs before
// the server stops: a test failing first leaves no goroutine held. The
// tests holding one boot on the real pool: on the driven clock every read
// waits for each connection and queue to catch up, held ones included.
type nameHold struct {
	entered     chan struct{}
	enter, free func()
	release     chan struct{}
}

func newNameHold() *nameHold {
	h := &nameHold{entered: make(chan struct{}), release: make(chan struct{})}
	h.enter = sync.OnceFunc(func() { close(h.entered) })
	h.free = sync.OnceFunc(func() { close(h.release) })
	return h
}

// wait is the hold itself: it reports it is entered and waits for free.
func (h *nameHold) wait() {
	h.enter()
	<-h.release
}

// TestAdminSetPlayerNameRefusedWhileCreateHoldsIt pins that //set name
// cannot take a name a character creation holds between its check and its
// insert: the rename is refused with INCORRECT_NAME_TRY_AGAIN, the
// creation goes through, and one row holds the name. Diverging from the
// reference, whose checks leave that window open, is owner-approved
// (#4765).
func TestAdminSetPlayerNameRefusedWhileCreateHoldsIt(t *testing.T) {
	t.Parallel()
	hold := newNameHold()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithRealPool(), gameservertest.WithCharacterCreateHold(func(name string) error {
		if name == "renamed7" {
			hold.wait()
		}
		return nil
	}))
	t.Cleanup(hold.free)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)
	creator := srv.DialClient(t, "creator", 0)

	creator.Send(encodeCreateCharacter("renamed7"))
	<-hold.entered
	frames := append(exchangeBarrier(t, gm, encodeBuildCmd("set name Renamed7")), settleBarrier(t, gm)...)
	if !incorrectName(frames) {
		t.Fatalf("GM frames = %x, want INCORRECT_NAME_TRY_AGAIN alone", testsupport.FrameOpcodes(frames))
	}
	hold.free()
	if reply := creator.Read(); reply[0] != serverpackets.OpcodeCharCreateOk {
		t.Fatalf("creation reply = %#x, want CharCreateOk", reply[0])
	}
	assertNotRenamed(t, srv, user, userID)
	if n := nameRows(t, srv, "Renamed7"); n != 1 {
		t.Fatalf("%d rows hold the name, want 1", n)
	}
}

// TestCreateRefusedWhileAdminSetPlayerNameHoldsIt pins that a character
// creation cannot take a name //set name holds between its check and its
// write: the creation is answered with the name-in-use reply, the rename
// goes through, and one row holds the name (#4765, owner-approved).
func TestCreateRefusedWhileAdminSetPlayerNameHoldsIt(t *testing.T) {
	t.Parallel()
	hold := newNameHold()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithRealPool(), gameservertest.WithCharacterNameFault(gameservertest.CharacterNameFault{StoreHold: hold.wait}))
	t.Cleanup(hold.free)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)
	creator := srv.DialClient(t, "creator", 0)

	gm.Send(encodeBuildCmd("set name Renamed7"))
	<-hold.entered
	creator.Send(encodeCreateCharacter("RENAMED7"))
	reply := creator.Read()
	if reply[0] != serverpackets.OpcodeCharCreateFail || reply[1] != byte(serverpackets.CharCreateFailReasonNameAlreadyExists) {
		t.Fatalf("creation reply = %x, want CharCreateFail name in use", reply)
	}
	hold.free()
	for {
		f := gm.Read()
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		if _, text := systemText(t, f); text != "You successfully set your target's name to Renamed7." {
			t.Fatalf("GM text = %q, want the rename reported", text)
		}
		break
	}
	if got := rowName(t, srv, userID); got != "Renamed7" {
		t.Fatalf("char_name = %q, want Renamed7", got)
	}
	if n := nameRows(t, srv, "Renamed7"); n != 1 {
		t.Fatalf("%d rows hold the name, want 1", n)
	}
}

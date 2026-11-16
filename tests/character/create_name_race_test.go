package character

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// nameRows counts the characters named name, matched case-insensitively
// as name lookups are.
func nameRows(t *testing.T, srv *gameservertest.Server, name string) int {
	t.Helper()
	var n int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM characters WHERE LOWER(char_name) = LOWER(?)", name).Scan(&n); err != nil {
		t.Fatalf("count %s rows: %v", name, err)
	}
	return n
}

// readCreateReply reads the answer to a RequestCharacterCreate already
// sent: the CharCreateFail reason, or -1 for CharCreateOk (whose refreshed
// CharSelectInfo it reads).
func readCreateReply(t *testing.T, c *testsupport.ScriptedClient, name string) int32 {
	t.Helper()
	reply := c.Read()
	switch reply[0] {
	case serverpackets.OpcodeCharCreateOk:
		if next := c.Read(); next[0] != serverpackets.OpcodeCharSelectInfo {
			t.Fatalf("%s: frame after CharCreateOk = %#x, want CharSelectInfo", name, next[0])
		}
		return -1
	case serverpackets.OpcodeCharCreateFail:
		return int32(reply[1]) | int32(reply[2])<<8 | int32(reply[3])<<16 | int32(reply[4])<<24
	default:
		t.Fatalf("%s: reply opcode = %#x, want CharCreateOk or CharCreateFail", name, reply[0])
		return 0
	}
}

// TestCreateSameNameConcurrently pins that two accounts creating one name
// at the same moment end with one character: one creation is answered
// CharCreateOk, the other the name-in-use reply, and one row holds the
// name. Diverging from the reference, whose check and insert leave a
// window two creations can both pass, is owner-approved (#4765).
func TestCreateSameNameConcurrently(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t)
	clients := []*testsupport.ScriptedClient{
		srv.DialClient(t, "racer1", 0),
		srv.DialClient(t, "racer2", 0),
	}

	for i := range 25 {
		name := fmt.Sprintf("Racer%d", i)
		payload := encodeRequestCharacterCreate(name, 0, 0, 0, 0, 0, 0)
		start := make(chan struct{})
		sendErrs := make([]error, len(clients))
		var wg sync.WaitGroup
		for j, c := range clients {
			wg.Go(func() {
				<-start
				sendErrs[j] = c.TrySend(payload)
			})
		}
		close(start)
		wg.Wait()
		if err := errors.Join(sendErrs...); err != nil {
			t.Fatalf("%s: send: %v", name, err)
		}

		created, inUse := 0, 0
		for _, c := range clients {
			switch got := readCreateReply(t, c, name); got {
			case -1:
				created++
			case int32(serverpackets.CharCreateFailReasonNameAlreadyExists):
				inUse++
			default:
				t.Fatalf("%s: reply = %d, want CharCreateOk or name in use", name, got)
			}
		}
		if created != 1 || inUse != 1 {
			t.Fatalf("%s: %d created and %d refused, want one of each", name, created, inUse)
		}
		if n := nameRows(t, srv, name); n != 1 {
			t.Fatalf("%s: %d rows hold the name, want 1", name, n)
		}
		// Keep both accounts under the character cap for the next round.
		if _, err := srv.DB.ExecContext(context.Background(), "DELETE FROM characters WHERE LOWER(char_name) = LOWER(?)", name); err != nil {
			t.Fatalf("%s: delete: %v", name, err)
		}
	}
}

// TestCreateNameFreedByFailedInsert pins that a creation whose insert
// fails holds its name no longer: the same name is created at the next
// try (#4765).
func TestCreateNameFreedByFailedInsert(t *testing.T) {
	t.Parallel()
	var failed bool
	srv := gameservertest.Boot(t, gameservertest.WithWantChars(0),
		gameservertest.WithCharacterCreateHold(func(name string) error {
			if name == "Faulty" && !failed {
				failed = true
				return errors.New("characters table down")
			}
			return nil
		}))
	c := srv.Client

	c.Send(encodeRequestCharacterCreate("Faulty", 0, 0, 0, 0, 0, 0))
	if got := readCreateReply(t, c, "Faulty"); got != int32(serverpackets.CharCreateFailReasonCreationFailed) {
		t.Fatalf("failed insert: reply = %d, want creation failed", got)
	}
	c.Send(encodeRequestCharacterCreate("Faulty", 0, 0, 0, 0, 0, 0))
	if got := readCreateReply(t, c, "Faulty"); got != -1 {
		t.Fatalf("retry: reply = %d, want CharCreateOk", got)
	}
	if n := nameRows(t, srv, "Faulty"); n != 1 {
		t.Fatalf("%d rows hold the name, want 1", n)
	}
}

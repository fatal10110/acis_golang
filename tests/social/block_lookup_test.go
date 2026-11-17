package social

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestBlockLookupHoldsNoQueue holds the characters table under a write
// lock while Alice blocks Bobby by name. While the name lookup waits on the
// database, Bobby's queue still runs work posted to it; once the lock is
// released the block lands and both players get their notices, in order.
// Were the lookup made on Alice's queue, the shared queue runner would wait
// on the lock until the lookup timed out and the block was refused.
func TestBlockLookupHoldsNoQueue(t *testing.T) {
	t.Parallel()
	p := bootPair(t)
	p.enterAll(t)
	ctx := context.Background()

	lock, err := p.srv.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := lock.ExecContext(ctx, `LOCK TABLES characters WRITE`); err != nil {
		t.Fatal(err)
	}
	unlocked := false
	unlock := func() {
		if !unlocked {
			unlocked = true
			if _, err := lock.ExecContext(ctx, `UNLOCK TABLES`); err != nil {
				t.Fatal(err)
			}
		}
	}
	defer unlock()

	p.alice.Send(encodeBlock(clientpackets.BlockAdd, "Bobby"))
	awaitLookupOnLock(t, p)
	ran := make(chan struct{})
	if !p.srv.PlayerQueue(t, p.bobbyID).Post(func() { close(ran) }) {
		t.Fatal("Bobby's queue refused the probe")
	}
	<-ran

	unlock()
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1AddedToYourIgnoreList, "Bobby")
	assertSystemMessageText(t, p.bobby.Read(), serverpackets.SystemMessageS1HasAddedYouToIgnoreList, "Alice")
	assertSilent(t, p.alice, "blocker after the notice")
	if !p.srv.Relations.IsBlocked(p.aliceID, p.bobbyID) {
		t.Fatal("the block did not land")
	}
}

// awaitLookupOnLock returns once the server's name lookup is waiting on the
// characters table lock.
func awaitLookupOnLock(t *testing.T, p *pair) {
	t.Helper()
	for {
		var n int
		if err := p.srv.DB.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM information_schema.PROCESSLIST
			  WHERE db = DATABASE() AND state LIKE 'Waiting for table%' AND info LIKE 'SELECT obj_Id, char_name%'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
	}
}

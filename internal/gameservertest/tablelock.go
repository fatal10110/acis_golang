package gameservertest

import (
	"context"
	"sync"
	"testing"
	"time"
)

// LockTable write-locks table from a database connection of its own, so
// every other statement on the table waits until release runs; cleanup
// releases it too. A suite holds a read the server makes open with it and
// pins what runs meanwhile. While a lock is held, a sent EnterWorld does not
// wait for the server to handle it (see awaitEnterWorld): the login may read
// the locked table.
func (s *Server) LockTable(tb testing.TB, table string) (release func()) {
	tb.Helper()
	return s.lockTable(tb, table, "WRITE")
}

// LockTableRead read-locks table like LockTable: reads of it go on, and
// every write waits until release runs.
func (s *Server) LockTableRead(tb testing.TB, table string) (release func()) {
	tb.Helper()
	return s.lockTable(tb, table, "READ")
}

// lockTable locks table in mode from a database connection of its own.
func (s *Server) lockTable(tb testing.TB, table, mode string) (release func()) {
	tb.Helper()
	ctx := context.Background()
	conn, err := s.DB.Conn(ctx)
	if err != nil {
		tb.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "LOCK TABLES "+table+" "+mode); err != nil {
		_ = conn.Close()
		tb.Fatal(err)
	}
	s.lockedTables.Add(1)
	var once sync.Once
	release = func() {
		once.Do(func() {
			defer s.lockedTables.Add(-1)
			if _, err := conn.ExecContext(ctx, `UNLOCK TABLES`); err != nil {
				tb.Errorf("unlock %s: %v", table, err)
			}
			_ = conn.Close()
		})
	}
	tb.Cleanup(release)
	return release
}

// AwaitLockWait returns once a statement starting with prefix waits on a
// table lock (see LockTable), failing tb when none does within 30 seconds.
func (s *Server) AwaitLockWait(tb testing.TB, prefix string) {
	tb.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			tb.Fatalf("no statement %q waited on the table lock", prefix)
		}
		var n int
		if err := s.DB.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM information_schema.PROCESSLIST
			  WHERE db = DATABASE() AND state LIKE 'Waiting for table%' AND info LIKE ?`, prefix+"%").Scan(&n); err != nil {
			tb.Fatal(err)
		}
		if n > 0 {
			return
		}
	}
}

// AwaitQueueRuns posts a probe to the live player objID's queue and returns
// once it has run: while a held read waits, it shows the read holds neither
// that queue nor the pool worker that would run it.
func (s *Server) AwaitQueueRuns(tb testing.TB, objID int32) {
	tb.Helper()
	ran := make(chan struct{})
	if !s.PlayerQueue(tb, objID).Post(func() { close(ran) }) {
		tb.Fatalf("the queue of %d refused the probe", objID)
	}
	<-ran
}

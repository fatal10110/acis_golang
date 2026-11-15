package lifecycle

import (
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/link"
)

// TestEvictedAuthLoginAnswerDoesNotAuthenticateReplacement holds the login
// server's answers so a second AuthLogin for the account, with keys the
// login server never issued, lands while the answer to the first is still
// in flight. The first connection is evicted; the login server's approval
// of its request must be dropped, and the replacement is refused on its
// own answer instead of entering on the evicted one's.
func TestEvictedAuthLoginAnswerDoesNotAuthenticateReplacement(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t)
	const account = "evictee"
	hold := srv.HoldLoginAnswers()

	issued := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}
	srv.IssueLoginSession(account, issued)
	first := srv.DialAuthLogin(t, account, issued)
	hold.Await(t, 1) // the login server approved the first request

	second := srv.DialAuthLogin(t, account, link.SessionKey{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4})
	if !first.AwaitClose(5 * time.Second) {
		t.Fatal("evicted connection still open")
	}
	hold.Await(t, 2) // and refused the second
	hold.Release()

	assertFrameOpcode(t, second.Read(), serverpackets.OpcodeAuthLoginFail, "replacement AuthLogin answer")
	assertFrameOpcode(t, second.Read(), serverpackets.OpcodeServerClose, "replacement ServerClose")
	second.ExpectClosed()
}

// TestOverlongAuthLoginNameIsRefusedAndLoginLinkStays sends AuthLogin
// names longer than any account the login server can issue a session for,
// one of them too long to fit a login-link frame at all: each is refused
// with AuthLoginFail, and an ordinary login right after still enters over
// the same login link.
func TestOverlongAuthLoginNameIsRefusedAndLoginLinkStays(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t)
	key := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}

	for _, name := range []string{strings.Repeat("a", 32755), strings.Repeat("a", 15)} {
		srv.IssueLoginSession(name, key)
		c := srv.DialAuthLogin(t, name, key)
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeAuthLoginFail, "over-long name AuthLogin answer")
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeServerClose, "over-long name ServerClose")
		c.ExpectClosed()
	}

	srv.DialClient(t, "afterlong", 0)
}

package lifecycle

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/link"
)

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

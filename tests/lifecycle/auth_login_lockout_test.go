package lifecycle

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestWrongKeyAuthLoginStreamCannotLockOwnerOut holds the login server's
// answers while a stream of AuthLogins presenting keys the login server
// never issued lands for an account, before and after the owner's
// AuthLogin with the issued keys. Every request reaches the login server
// and every client gets its own answer in send order: each wrong-key
// connection reads AuthLoginFail and ServerClose, and the owner, neither
// refused nor displaced by them, enters on its own approval (#5242). A
// pending login is still never displaced by an unapproved one: owner-
// approved divergence (#4652).
func TestWrongKeyAuthLoginStreamCannotLockOwnerOut(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t)
	const account = "lockowner"
	hold := srv.HoldLoginAnswers()

	issued := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}
	srv.IssueLoginSession(account, issued)
	sent := 0
	dial := func(key link.SessionKey) *testsupport.ScriptedClient {
		c := srv.DialAuthLogin(t, account, key)
		sent++
		hold.Await(t, sent) // the login server answered it, in send order
		return c
	}
	var wrong []*testsupport.ScriptedClient
	dialWrong := func() {
		k := int32(len(wrong) + 1)
		wrong = append(wrong, dial(link.SessionKey{LoginKey1: k, LoginKey2: k, PlayKey1: k, PlayKey2: k}))
	}

	for range 5 {
		dialWrong()
	}
	owner := dial(issued)
	for range 2 {
		dialWrong()
	}
	hold.Release()

	for _, c := range wrong {
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeAuthLoginFail, "wrong-key AuthLogin answer")
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeServerClose, "wrong-key AuthLogin ServerClose")
		c.ExpectClosed()
	}
	assertFrameOpcode(t, owner.Read(), serverpackets.OpcodeCharSelectInfo, "owner AuthLogin answer")
}

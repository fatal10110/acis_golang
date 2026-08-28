package network

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// waitForPendingValidation polls until account has an in-flight validation
// registered with v, so the test can drive the second AuthLogin exactly
// while the first is still pending — the reachable precondition from
// LoginServerThread.java:292-304 (issue #2040) — instead of racing goroutines
// against real network timing.
func waitForPendingValidation(t *testing.T, v *SessionValidator, account string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		v.mu.Lock()
		_, pending := v.waiting[account]
		v.mu.Unlock()
		if pending {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("validation for %q never became pending", account)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestDuplicateAuthLoginEvictsPendingConnectionAndValidatesReplacement pins
// issue #2040: a second AuthLogin for an account whose first validation is
// still pending evicts the first connection and validates the second,
// matching LoginServerThread.addClient (LoginServerThread.java:292-304)
// instead of rejecting the second connection outright.
func TestDuplicateAuthLoginEvictsPendingConnectionAndValidatesReplacement(t *testing.T) {
	// No internal timeout: every login-server answer in this test is driven
	// by hand via validator.Resolve, so a real deadline would only mask a
	// broken wait instead of exercising it.
	validator := newSessionValidator(0)

	// A bare LoginLink whose peer drains every write and never answers on
	// its own, so both AuthLogin requests stay pending until this test
	// resolves them itself — the deterministic version of "before the
	// login-server response to the first PlayerAuthRequest".
	loginConn, loginPeer := net.Pipe()
	go io.Copy(io.Discard, loginPeer)
	t.Cleanup(func() { loginConn.Close(); loginPeer.Close() })
	loginLink := &LoginLink{
		conn:   loginConn,
		crypt:  crypt.NewLinkCrypt(),
		log:    zerolog.Nop(),
		done:   make(chan struct{}),
		frames: wire.NewFrameReader(loginConn),
	}
	t.Cleanup(func() { loginLink.Close() })

	addr, chars, _, _, _ := newTestGameClientLinkWithSkillsShortcutsAndLog(t, func() *LoginLink { return loginLink }, validator, nil, zerolog.Nop())
	seedSelectableCharacter(t, chars, "player1", "Newbie", 1, 0)

	key := link.SessionKey{LoginKey1: 11, LoginKey2: 22, PlayKey1: 33, PlayKey2: 44}

	a := testsupport.Dial(t, addr)
	a.SendProtocolVersion(746)
	a.Send(encodeAuthLogin("player1", key))
	waitForPendingValidation(t, validator, "player1")

	b := testsupport.Dial(t, addr)
	b.SendProtocolVersion(746)
	b.Send(encodeAuthLogin("player1", key))

	// A never gets an answer of its own: it is evicted and disconnected
	// while its request is still outstanding.
	if !a.AwaitClose(2 * time.Second) {
		t.Fatal("evicted connection did not close")
	}

	// B's request is the one now in flight; resolving it as a login-server
	// success lets B finish authenticating exactly like a fresh connection
	// would, proving the account is usable again right away.
	waitForPendingValidation(t, validator, "player1")
	validator.Resolve("player1", true)

	reply := b.Read()
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
}

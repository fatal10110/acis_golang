package gameservertest

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// loginAnswerQueue holds the login server's PlayerAuthResponse answers
// while a test decides when the game server reads them. mu guards every
// field and also orders delivery: an answer arriving while held ones are
// released waits for them, so answers reach the game server in the order
// the login server sent them.
type loginAnswerQueue struct {
	mu      sync.Mutex
	holding bool
	held    []heldLoginAnswer
	resolve func(string, bool)
	arrived chan struct{}
}

type heldLoginAnswer struct {
	account string
	ok      bool
}

func (q *loginAnswerQueue) deliver(account string, ok bool, resolve func(string, bool)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.holding {
		resolve(account, ok)
		return
	}
	q.resolve = resolve
	q.held = append(q.held, heldLoginAnswer{account: account, ok: ok})
	select {
	case q.arrived <- struct{}{}:
	default:
	}
}

// LoginAnswerHold keeps the login server's answers to session checks from
// reaching the game server until Release.
type LoginAnswerHold struct {
	q *loginAnswerQueue
}

// HoldLoginAnswers starts holding the login server's answers to session
// checks: each one queues, in arrival order, until the returned hold's
// Release delivers them.
func (s *Server) HoldLoginAnswers() *LoginAnswerHold {
	q := &s.shutdown.hold
	q.mu.Lock()
	defer q.mu.Unlock()
	q.holding = true
	q.arrived = make(chan struct{}, 1)
	return &LoginAnswerHold{q: q}
}

// Await blocks until n answers are held, failing tb if they do not arrive
// within loginLinkSyncWait (a failure guard, not a pacing delay).
func (h *LoginAnswerHold) Await(tb testing.TB, n int) {
	tb.Helper()
	deadline := time.After(loginLinkSyncWait)
	for {
		h.q.mu.Lock()
		held := len(h.q.held)
		h.q.mu.Unlock()
		if held >= n {
			return
		}
		select {
		case <-h.q.arrived:
		case <-deadline:
			tb.Fatalf("login server answered %d session checks, want %d", held, n)
		}
	}
}

// Release delivers every held answer in arrival order and stops holding.
func (h *LoginAnswerHold) Release() {
	q := h.q
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, a := range q.held {
		q.resolve(a.account, a.ok)
	}
	q.held, q.holding = nil, false
}

// IssueLoginSession stores key as account's session on the login server,
// as if the account had just logged in and picked this server.
func (s *Server) IssueLoginSession(account string, key link.SessionKey) {
	s.sessions.Put(account, key)
}

// DialAuthLogin connects a scripted client, completes the protocol
// handshake and sends AuthLogin for account presenting key, without
// reading the answer.
func (s *Server) DialAuthLogin(t *testing.T, account string, key link.SessionKey) *testsupport.ScriptedClient {
	t.Helper()
	c := testsupport.Dial(t, s.addr.String())
	c.SendProtocolVersion(746)
	c.Send(encodeAuthLogin(account, key))
	return c
}

// encodeAuthLogin builds an AuthLogin payload, play keys in the client's
// reversed order.
func encodeAuthLogin(account string, key link.SessionKey) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAuthLogin)
	w.WriteString(account)
	w.WriteInt32(key.PlayKey2)
	w.WriteInt32(key.PlayKey1)
	w.WriteInt32(key.LoginKey1)
	w.WriteInt32(key.LoginKey2)
	return w.Bytes()
}

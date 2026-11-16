package network

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/link"
)

// DefaultValidationTimeout bounds how long a game-client goroutine waits
// for the login server to answer a session validation request.
const DefaultValidationTimeout = 5 * time.Second

var (
	errValidationPending = errors.New("session validation already pending")
	errLoginLinkClosed   = errors.New("login link closed during session validation")
)

// SessionValidator confirms game clients' presented session keys with the
// login server over a LoginLink. A game server keeps exactly one link to
// the login server at a time but serves many clients concurrently, so
// SessionValidator correlates each client's in-flight request with the
// login server's later, asynchronous response.
//
// A response names only the account, and the login server answers a
// link's requests in the order it reads them, so responses are matched to
// requests per account in send order. A request whose client stopped
// waiting (evicted by a kick, timed out, canceled) keeps its place
// in that order until its response arrives and is dropped: the response
// can never reach a later request for the same account.
//
// sendMu makes queueing a request and writing it one step, so each
// account's queue order is the wire order. mu guards pending and every
// queued request's abandoned flag.
type SessionValidator struct {
	sendMu  sync.Mutex
	mu      sync.Mutex
	pending map[string][]*authRequest
	timeout time.Duration
}

// authRequest is one session check awaiting the login server's answer.
type authRequest struct {
	// linkDone is the Done channel of the link the request went out on: a
	// closed link answers nothing more, so its requests are dropped.
	linkDone <-chan struct{}
	// result receives the one answer for the waiting client: the login
	// server's, or the rejection an eviction delivers.
	result chan bool
	// abandoned marks a request no client waits on; its answer is dropped.
	abandoned bool
}

// NewSessionValidator returns a SessionValidator with no requests
// outstanding.
func NewSessionValidator() *SessionValidator {
	return newSessionValidator(DefaultValidationTimeout)
}

func newSessionValidator(timeout time.Duration) *SessionValidator {
	return &SessionValidator{pending: make(map[string][]*authRequest), timeout: timeout}
}

// Resolve delivers ok as the login server's answer to accountName's oldest
// outstanding request. Assign this as a LoginLink's
// LoginLinkHandlers.PlayerAuthResponse.
func (v *SessionValidator) Resolve(accountName string, ok bool) {
	v.ResolveThen(accountName, ok, nil)
}

// ResolveThen is Resolve that, when the answer is an approval reaching a
// client still waiting, runs approved before that client is released. An
// answer to a request whose client is gone runs nothing.
func (v *SessionValidator) ResolveThen(accountName string, ok bool, approved func()) {
	v.mu.Lock()
	queue := v.liveQueue(accountName)
	if len(queue) == 0 {
		v.mu.Unlock()
		return
	}
	req := queue[0]
	v.setQueue(accountName, queue[1:])
	abandoned := req.abandoned
	v.mu.Unlock()

	// Once dequeued, req is reachable from this call alone: neither Evict
	// nor the waiter's own abandon touches result any more.
	if abandoned {
		return
	}
	if ok && approved != nil {
		approved()
	}
	req.result <- ok
}

// Evict rejects accountName's outstanding validation, if any, so its client
// stops waiting at once. The request keeps its place in the queue, so the
// login server's answer to it is dropped instead of reaching a later
// request for the account.
func (v *SessionValidator) Evict(accountName string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, req := range v.pending[accountName] {
		if !req.abandoned {
			req.abandoned = true
			req.result <- false
		}
	}
}

// liveQueue returns accountName's queue without the requests sent on a link
// that has since closed. The caller holds mu.
func (v *SessionValidator) liveQueue(accountName string) []*authRequest {
	queue := v.pending[accountName]
	live := queue[:0]
	for _, req := range queue {
		select {
		case <-req.linkDone:
		default:
			live = append(live, req)
		}
	}
	clear(queue[len(live):])
	v.setQueue(accountName, live)
	return live
}

func (v *SessionValidator) setQueue(accountName string, queue []*authRequest) {
	if len(queue) == 0 {
		delete(v.pending, accountName)
		return
	}
	v.pending[accountName] = queue
}

// enqueue appends req to accountName's queue unless a client is already
// waiting on a request for that account. The caller holds sendMu.
func (v *SessionValidator) enqueue(accountName string, req *authRequest) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	queue := v.liveQueue(accountName)
	for _, queued := range queue {
		if !queued.abandoned {
			return false
		}
	}
	v.pending[accountName] = append(queue, req)
	return true
}

// remove drops req, which never reached the login server, from
// accountName's queue. The caller holds sendMu, so req is still the
// newest entry and removing it keeps the queue in wire order.
func (v *SessionValidator) remove(accountName string, req *authRequest) {
	v.mu.Lock()
	defer v.mu.Unlock()
	queue := v.pending[accountName]
	for i, queued := range queue {
		if queued == req {
			v.setQueue(accountName, append(queue[:i], queue[i+1:]...))
			return
		}
	}
}

// abandon marks req as no longer waited on. A request still queued keeps
// its place until the login server answers it.
func (v *SessionValidator) abandon(req *authRequest) {
	v.mu.Lock()
	req.abandoned = true
	v.mu.Unlock()
}

// Validate asks the login server, over loginLink, to confirm the session
// keys req presented, then blocks until it answers or ctx is done.
//
// A confirmed match advances client to StateAuthed and records its session
// key, and Validate reports (true, nil). A confirmed mismatch sends client
// an AuthLoginFail notice and reports (false, nil); the caller is
// responsible for closing the connection afterward, same as for any other
// definitive rejection. A transport error or a canceled ctx reports
// (false, err) without notifying client, since no definitive answer came
// back.
func (v *SessionValidator) Validate(ctx context.Context, client *Client, req clientpackets.AuthLogin, loginLink *LoginLink) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	pending := &authRequest{linkDone: loginLink.Done(), result: make(chan bool, 1)}
	v.sendMu.Lock()
	if !v.enqueue(req.LoginName, pending) {
		v.sendMu.Unlock()
		return false, errValidationPending
	}
	err := loginLink.SendPlayerAuthRequest(link.PlayerAuthRequest{
		Account: req.LoginName,
		SessionKey: link.SessionKey{
			PlayKey1:  req.PlayKey1,
			PlayKey2:  req.PlayKey2,
			LoginKey1: req.LoginKey1,
			LoginKey2: req.LoginKey2,
		},
	})
	if err != nil {
		v.remove(req.LoginName, pending)
	}
	v.sendMu.Unlock()
	if err != nil {
		return false, err
	}
	defer v.abandon(pending)
	result := pending.result

	waitCtx := ctx
	cancel := func() {}
	if v.timeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, v.timeout)
	}
	defer cancel()

	// waitCtx takes priority over an already-available result: once the caller
	// has given up, Validate must not act on a late answer, since the
	// caller may already be tearing down client's connection.
	select {
	case <-waitCtx.Done():
		return false, waitCtx.Err()
	default:
	}

	select {
	case ok := <-result:
		// waitCtx.Err() is re-checked here, not just above: waitCtx and result
		// can become ready in the same instant, and Go's select then picks
		// pseudo-randomly between this case and the case below. Deciding
		// precedence again at the point of consuming result, rather than
		// trusting which case fired, is what makes cancellation win either way.
		switch outcome := decideValidation(waitCtx.Err(), ok); outcome {
		case validationCanceled:
			return false, waitCtx.Err()
		case validationRejected:
			client.Session.SendFrame(serverpackets.FrameAuthLoginFail(serverpackets.LoginFailSystemErrorTryLater))
			return false, nil
		default:
			client.SetAuthenticated(req.LoginName, link.SessionKey{
				LoginKey1: req.LoginKey1,
				LoginKey2: req.LoginKey2,
				PlayKey1:  req.PlayKey1,
				PlayKey2:  req.PlayKey2,
			})
			return true, nil
		}
	case <-loginLink.Done():
		return false, errLoginLinkClosed
	case <-waitCtx.Done():
		return false, waitCtx.Err()
	}
}

// validationOutcome resolves the precedence between a login-server answer
// and cancellation when both are available at once: cancellation always
// wins, even over a positive match.
type validationOutcome int

const (
	validationCanceled validationOutcome = iota
	validationRejected
	validationApproved
)

func decideValidation(waitCtxErr error, ok bool) validationOutcome {
	if waitCtxErr != nil {
		return validationCanceled
	}
	if !ok {
		return validationRejected
	}
	return validationApproved
}

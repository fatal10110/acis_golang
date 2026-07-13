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
// login server's later, asynchronous response by account name.
//
// mu guards waiting.
type SessionValidator struct {
	mu      sync.Mutex
	waiting map[string]chan bool
	timeout time.Duration
}

// NewSessionValidator returns a SessionValidator with no requests
// outstanding.
func NewSessionValidator() *SessionValidator {
	return newSessionValidator(DefaultValidationTimeout)
}

func newSessionValidator(timeout time.Duration) *SessionValidator {
	return &SessionValidator{waiting: make(map[string]chan bool), timeout: timeout}
}

// Resolve delivers ok as the login server's answer for accountName's
// outstanding validation, if one exists, and forgets it. Assign this as a
// LoginLink's LoginLinkHandlers.PlayerAuthResponse.
func (v *SessionValidator) Resolve(accountName string, ok bool) {
	v.mu.Lock()
	ch, found := v.waiting[accountName]
	delete(v.waiting, accountName)
	v.mu.Unlock()

	if found {
		ch <- ok
	}
}

func (v *SessionValidator) register(accountName string) (<-chan bool, bool) {
	ch := make(chan bool, 1)
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, exists := v.waiting[accountName]; exists {
		return nil, false
	}
	v.waiting[accountName] = ch
	return ch, true
}

func (v *SessionValidator) forget(accountName string) {
	v.mu.Lock()
	delete(v.waiting, accountName)
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

	result, ok := v.register(req.LoginName)
	if !ok {
		return false, errValidationPending
	}
	defer v.forget(req.LoginName)

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
		return false, err
	}

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
		if !ok {
			client.Session.SendFrame(serverpackets.FrameAuthLoginFail(serverpackets.LoginFailSystemErrorTryLater))
			return false, nil
		}
		client.SetAuthenticated(req.LoginName, link.SessionKey{
			LoginKey1: req.LoginKey1,
			LoginKey2: req.LoginKey2,
			PlayKey1:  req.PlayKey1,
			PlayKey2:  req.PlayKey2,
		})
		return true, nil
	case <-loginLink.Done():
		return false, errLoginLinkClosed
	case <-waitCtx.Done():
		return false, waitCtx.Err()
	}
}

package network

import (
	"context"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/loginserver/link"
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
}

// NewSessionValidator returns a SessionValidator with no requests
// outstanding.
func NewSessionValidator() *SessionValidator {
	return &SessionValidator{waiting: make(map[string]chan bool)}
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

func (v *SessionValidator) register(accountName string) <-chan bool {
	ch := make(chan bool, 1)
	v.mu.Lock()
	v.waiting[accountName] = ch
	v.mu.Unlock()
	return ch
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
	result := v.register(req.LoginName)

	err := loginLink.SendPlayerAuthRequest(link.PlayerAuthRequest{
		Account:   req.LoginName,
		PlayKey1:  req.PlayKey1,
		PlayKey2:  req.PlayKey2,
		LoginKey1: req.LoginKey1,
		LoginKey2: req.LoginKey2,
	})
	if err != nil {
		v.forget(req.LoginName)
		return false, err
	}

	select {
	case ok := <-result:
		if !ok {
			client.Session.Send(serverpackets.EncodeAuthLoginFail(serverpackets.LoginFailSystemErrorTryLater))
			return false, nil
		}
		client.Authenticate(req.LoginName, SessionKey{
			LoginKey1: req.LoginKey1,
			LoginKey2: req.LoginKey2,
			PlayKey1:  req.PlayKey1,
			PlayKey2:  req.PlayKey2,
		}, func(string) bool { return true })
		return true, nil
	case <-ctx.Done():
		v.forget(req.LoginName)
		return false, ctx.Err()
	}
}

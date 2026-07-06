package network

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
)

// dialTestLoginLink establishes a real LoginLink against a freshly started
// test login server, wiring validator.Resolve as its PlayerAuthResponse
// handler so SessionValidator.Validate gets a genuine round trip.
func dialTestLoginLink(t *testing.T, validator *SessionValidator) (link *LoginLink, sessions *manager.SessionStore) {
	t.Helper()

	addr, servers, sessions := newTestLoginServer(t, false)
	servers.Register(1, testHexID)

	auth := LoginServerAuth{ServerID: 1, HexID: testHexID, HostName: "*", Port: 7777, MaxPlayers: 300}
	handlers := LoginLinkHandlers{PlayerAuthResponse: validator.Resolve}

	l, err := DialLoginLink(context.Background(), addr, auth, handlers, nil)
	if err != nil {
		t.Fatalf("DialLoginLink: %v", err)
	}
	t.Cleanup(func() { l.Close() })

	return l, sessions
}

func TestSessionValidatorValidateAdvancesClientOnMatch(t *testing.T) {
	validator := NewSessionValidator()
	loginLink, sessions := dialTestLoginLink(t, validator)

	sessions.Put("player1", manager.SessionKey{LoginKey1: 33, LoginKey2: 44, PlayKey1: 22, PlayKey2: 11})

	client := NewClient(nil)
	req := clientpackets.AuthLogin{LoginName: "player1", PlayKey1: 22, PlayKey2: 11, LoginKey1: 33, LoginKey2: 44}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ok, err := validator.Validate(ctx, client, req, loginLink)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !ok {
		t.Fatal("Validate() = false, want true for matching session key")
	}
	if got := client.State(); got != StateAuthed {
		t.Fatalf("client state = %s, want %s", got, StateAuthed)
	}
	if got := client.AccountName(); got != "player1" {
		t.Fatalf("client account name = %q, want %q", got, "player1")
	}
	want := SessionKey{LoginKey1: 33, LoginKey2: 44, PlayKey1: 22, PlayKey2: 11}
	if got := client.SessionKey(); got != want {
		t.Fatalf("client session key = %+v, want %+v", got, want)
	}
}

func TestSessionValidatorValidateRejectsMismatchAndNotifiesClient(t *testing.T) {
	validator := NewSessionValidator()
	loginLink, sessions := dialTestLoginLink(t, validator)

	// A session is stored, but under different key values than the client
	// presents, so the login server reports a mismatch.
	sessions.Put("player1", manager.SessionKey{LoginKey1: 1, LoginKey2: 2, PlayKey1: 3, PlayKey2: 4})

	session, rawClientConn := pipeSessions(t)
	client := NewClient(session)
	req := clientpackets.AuthLogin{LoginName: "player1", PlayKey1: 999, PlayKey2: 888, LoginKey1: 777, LoginKey2: 666}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ok, err := validator.Validate(ctx, client, req, loginLink)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if ok {
		t.Fatal("Validate() = true, want false for mismatching session key")
	}
	if got := client.State(); got != StateConnected {
		t.Fatalf("client state = %s, want %s (unchanged)", got, StateConnected)
	}

	// Validate's failure notice is the first (and only) packet sent on this
	// session, so the cipher has not armed yet and the frame is cleartext.
	frame := make([]byte, frameHeaderSize+len(serverpackets.EncodeAuthLoginFail(serverpackets.LoginFailSystemErrorTryLater)))
	if _, err := io.ReadFull(rawClientConn, frame); err != nil {
		t.Fatalf("read AuthLoginFail frame: %v", err)
	}
	if got := binary.LittleEndian.Uint16(frame); got != uint16(len(frame)) {
		t.Fatalf("frame length header = %d, want %d", got, len(frame))
	}
	want := serverpackets.EncodeAuthLoginFail(serverpackets.LoginFailSystemErrorTryLater)
	if got := frame[frameHeaderSize:]; !bytes.Equal(got, want) {
		t.Fatalf("AuthLoginFail payload = % X, want % X", got, want)
	}
}

func TestSessionValidatorValidateReturnsErrorOnContextCancel(t *testing.T) {
	validator := NewSessionValidator()
	loginLink, _ := dialTestLoginLink(t, validator)

	// No PlayerAuthResponse ever arrives for this account, since the login
	// server only answers requests it actually received; canceling ctx
	// must still return promptly instead of blocking forever.
	client := NewClient(nil)
	req := clientpackets.AuthLogin{LoginName: "nobody"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ok, err := validator.Validate(ctx, client, req, loginLink)
	if err == nil {
		t.Fatal("Validate: want error on canceled context, got nil")
	}
	if ok {
		t.Fatal("Validate() = true, want false on canceled context")
	}
}

package network

import (
	"net"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestAdminBanAccountOfLeavingPlayerReachesLogin pins that //ban account on
// a player already leaving the world still sends the ban to the login
// server: the target's queue is closed by then, so a request posted there
// would be dropped while the GM is told the account is banned.
func TestAdminBanAccountOfLeavingPlayerReachesLogin(t *testing.T) {
	loginConn, loginPeer := net.Pipe()
	t.Cleanup(func() { loginConn.Close(); loginPeer.Close() })
	loginLink := &LoginLink{
		conn:   loginConn,
		crypt:  crypt.NewLinkCrypt(),
		log:    zerolog.Nop(),
		done:   make(chan struct{}),
		frames: wire.NewFrameReader(loginConn),
	}

	received := make(chan link.ChangeAccessLevel, 1)
	go func() {
		frame, err := wire.NewFrameReader(loginPeer).ReadFrame()
		if err != nil {
			return
		}
		payload := append([]byte(nil), frame...)
		if err := crypt.NewLinkCrypt().Decrypt(payload); err != nil {
			t.Errorf("decrypt login frame: %v", err)
			return
		}
		req, err := link.DecodeChangeAccessLevel(payload)
		if err != nil {
			t.Errorf("decode login frame: %v", err)
			return
		}
		received <- req
	}()

	state := world.New()
	gmCap := &testsupport.FrameCapture{}
	gm := newTestLivePlayer(t, 1, gmCap)
	gm.Name = "Admin"
	target := newTestLivePlayer(t, 2, &testsupport.FrameCapture{})
	target.Name = "Leaver"
	target.Character.AccountName = "leaveracct"
	state.Spawn(target, 100, 0, 0, 0)
	state.AddPlayer(target)
	// Detach has begun: the target is still found by name, but its queue
	// refuses new tasks.
	target.Queue().Close()

	l := &GameClientLink{world: state, log: zerolog.Nop(), loginLink: func() *LoginLink { return loginLink }}
	l.adminBan(gm, "admin_ban account Leaver")

	select {
	case req := <-received:
		if req.Account != "leaveracct" || req.Level != accountBannedLevel {
			t.Fatalf("login server got %+v, want account leaveracct level %d", req, accountBannedLevel)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("login server never received the account ban")
	}
}

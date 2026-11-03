package botclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Counters are packet totals shared by every connection of a swarm.
type Counters struct {
	Sent, Received atomic.Int64
}

// GameConn is the client side of one game-server connection. One goroutine
// may Send while another reads: the cipher keeps separate inbound and
// outbound keys, and both are armed before Dial returns.
type GameConn struct {
	conn     net.Conn
	frames   *wire.FrameReader
	cipher   *gamecipher.Cipher // nil when the server left crypt off
	timeout  time.Duration
	counters *Counters
}

// DialGame connects to a game server and completes the cleartext
// ProtocolVersion/VersionCheck exchange that arms the connection's cipher.
// Each later Send waits at most timeout; counters may be nil.
func DialGame(ctx context.Context, addr string, revision int32, timeout time.Duration, counters *Counters) (*GameConn, error) {
	if counters == nil {
		counters = &Counters{}
	}
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial game server: %w", err)
	}
	g := &GameConn{conn: conn, frames: wire.NewFrameReader(conn), timeout: timeout, counters: counters}
	if err := g.handshake(revision); err != nil {
		conn.Close()
		return nil, err
	}
	return g, nil
}

func (g *GameConn) handshake(revision int32) error {
	if err := g.Send(encodeProtocolVersion(revision)); err != nil {
		return fmt.Errorf("send ProtocolVersion: %w", err)
	}
	raw, err := g.Read(time.Now().Add(g.timeout))
	if err != nil {
		return fmt.Errorf("read VersionCheck: %w", err)
	}
	key, enabled, err := decodeVersionCheck(raw)
	if err != nil {
		return err
	}
	if enabled {
		c, err := gamecipher.NewCipher(key)
		if err != nil {
			return err
		}
		c.Encrypt(nil) // arms both directions: VersionCheck itself was cleartext
		g.cipher = c
	}
	return nil
}

// Send encrypts a copy of payload and writes it as one frame.
func (g *GameConn) Send(payload []byte) error {
	buf := append([]byte(nil), payload...)
	if g.cipher != nil {
		g.cipher.Encrypt(buf)
	}
	if err := g.conn.SetWriteDeadline(time.Now().Add(g.timeout)); err != nil {
		return err
	}
	if err := wire.WriteFrame(g.conn, buf); err != nil {
		return err
	}
	g.counters.Sent.Add(1)
	return nil
}

// Read waits until deadline (zero: forever) for one frame and returns it
// decrypted.
func (g *GameConn) Read(deadline time.Time) ([]byte, error) {
	if err := g.conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	payload, err := g.frames.ReadFrame()
	if err != nil {
		return nil, err
	}
	if g.cipher != nil {
		g.cipher.Decrypt(payload)
	}
	if len(payload) == 0 {
		return nil, errors.New("empty game frame")
	}
	g.counters.Received.Add(1)
	return payload, nil
}

// Close closes the connection; a blocked Read returns.
func (g *GameConn) Close() error { return g.conn.Close() }

// Self is the bot's own character as the EnterWorld burst described it.
type Self struct {
	ObjectID int32
	X, Y, Z  int32
	Heading  int32
	// Created reports that EnterGame created the character and so waited
	// out the select delay.
	Created bool
	// Burst holds copies of the EnterWorld frames that preceded UserInfo
	// (objects already in sight among them), in arrival order.
	Burst [][]byte
}

// GameStartError reports a game-server rejection before the world was entered.
type GameStartError struct {
	Packet string
	Reason int32
}

func (e *GameStartError) Error() string {
	return fmt.Sprintf("game server replied %s (reason %d)", e.Packet, e.Reason)
}

// EnterGame authenticates account with the keys its login session issued,
// selects the first character (creating charName when the account has none,
// then waiting selectWait, the server's character-select reuse delay, which
// drops a game start sent sooner), and enters the world. It returns once the
// burst's UserInfo arrives, with the burst frames before it; the rest of the
// burst is left unread.
func EnterGame(ctx context.Context, g *GameConn, account string, keys SessionKeys, charName string, selectWait time.Duration) (Self, error) {
	if err := g.Send(encodeAuthLogin(account, keys)); err != nil {
		return Self{}, fmt.Errorf("send AuthLogin: %w", err)
	}
	reply, err := g.readUntil(serverpackets.OpcodeCharSelectInfo, serverpackets.OpcodeAuthLoginFail)
	if err != nil {
		return Self{}, fmt.Errorf("read CharSelectInfo: %w", err)
	}
	if reply[0] == serverpackets.OpcodeAuthLoginFail {
		return Self{}, &GameStartError{Packet: "AuthLoginFail", Reason: wire.NewReader(reply[1:]).ReadInt32()}
	}
	created := wire.NewReader(reply[1:]).ReadInt32() == 0
	if created {
		if err := g.Send(encodeRequestCharacterCreate(charName)); err != nil {
			return Self{}, fmt.Errorf("send RequestCharacterCreate: %w", err)
		}
		reply, err := g.readUntil(serverpackets.OpcodeCharCreateOk, serverpackets.OpcodeCharCreateFail)
		if err != nil {
			return Self{}, fmt.Errorf("read CharCreateOk: %w", err)
		}
		if reply[0] == serverpackets.OpcodeCharCreateFail {
			return Self{}, &GameStartError{Packet: "CharCreateFail", Reason: wire.NewReader(reply[1:]).ReadInt32()}
		}
		if _, err := g.readUntil(serverpackets.OpcodeCharSelectInfo); err != nil {
			return Self{}, fmt.Errorf("read CharSelectInfo after create: %w", err)
		}
		if err := wait(ctx, selectWait); err != nil {
			return Self{}, err
		}
	}

	if err := g.Send(encodeRequestGameStart(0)); err != nil {
		return Self{}, fmt.Errorf("send RequestGameStart: %w", err)
	}
	if _, err := g.readUntil(serverpackets.OpcodeCharSelected); err != nil {
		return Self{}, fmt.Errorf("read CharSelected: %w", err)
	}
	if err := g.Send(encodeEnterWorld()); err != nil {
		return Self{}, fmt.Errorf("send EnterWorld: %w", err)
	}
	var burst [][]byte
	deadline := time.Now().Add(g.timeout)
	for {
		raw, err := g.Read(deadline)
		if err != nil {
			return Self{}, fmt.Errorf("read UserInfo: %w", err)
		}
		switch raw[0] {
		case serverpackets.OpcodeUserInfo:
			self := decodeUserInfo(raw)
			self.Created, self.Burst = created, burst
			return self, nil
		case serverpackets.OpcodeServerClose:
			return Self{}, errServerClose
		}
		burst = append(burst, append([]byte(nil), raw...))
	}
}

// readUntil skips frames until one with any of opcodes arrives, all within
// one timeout budget.
func (g *GameConn) readUntil(opcodes ...byte) ([]byte, error) {
	deadline := time.Now().Add(g.timeout)
	for {
		reply, err := g.Read(deadline)
		if err != nil {
			return nil, err
		}
		for _, op := range opcodes {
			if reply[0] == op {
				return reply, nil
			}
		}
		if reply[0] == serverpackets.OpcodeServerClose {
			return nil, errServerClose
		}
	}
}

var errServerClose = errors.New("server sent ServerClose")

// wait blocks for d or until ctx ends.
func wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

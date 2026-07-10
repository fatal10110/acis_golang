package network

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
)

// dynamicKeySize is the fixed length in bytes of the Blowfish key this game
// server generates for the GS-LS link once the bootstrap handshake
// completes.
const dynamicKeySize = 40

// linkPublicExponent is the RSA public exponent the login server always
// uses for its GS-LS link key pair (F4/65537); InitLS carries only the
// modulus, so the game server assumes this fixed exponent to rebuild the
// public key.
const linkPublicExponent = 65537

// DefaultReconnectDelay is the reconnect delay a caller would normally pass
// to Maintain between failed or ended link attempts.
const DefaultReconnectDelay = 10 * time.Second

// generateDynamicKey reads a dynamicKeySize-byte Blowfish key from src,
// redrawing if the first byte is zero. The login server recovers this key by
// RSA-decrypting it with no padding and stripping leading zero bytes from
// the result; a leading zero byte here would make it strip a byte we didn't
// intend to lose, leaving the two sides with different-length keys.
// Redrawing is cheap: the odds of it triggering are 1 in 256.
func generateDynamicKey(src io.Reader) ([]byte, error) {
	key := make([]byte, dynamicKeySize)
	for {
		if _, err := io.ReadFull(src, key); err != nil {
			return nil, err
		}
		if key[0] != 0 {
			return key, nil
		}
	}
}

// LoginServerAuth is the identity a game server presents when registering
// on the login server's GS-LS link: the server id it wants, its hex auth
// key, and the address/capacity it advertises to clients through the login
// server's server list.
type LoginServerAuth struct {
	ServerID          int
	AcceptAlternateID bool
	HexID             []byte
	HostName          string // "*" lets the login server use the observed connection address
	Port              uint16
	ReserveHost       bool
	MaxPlayers        int32
}

// LoginLinkHandlers are a game server's callbacks for messages the login
// server sends after a LoginLink is established. A nil field is ignored.
type LoginLinkHandlers struct {
	// PlayerAuthResponse reports whether account's session keys, presented
	// in a prior SendPlayerAuthRequest, were valid.
	PlayerAuthResponse func(account string, ok bool)
	// KickPlayer requests that account be disconnected from this game
	// server.
	KickPlayer func(account string)
}

// LoginLink is this game server's established connection to the login
// server over the GS-LS link protocol. Past the handshake performed by
// DialLoginLink, it carries the confirmed registration identity and lets
// callers send further protocol messages; a background goroutine dispatches
// inbound ones to LoginLinkHandlers until the connection closes.
//
// send is the only thing allowed to write conn or advance crypt on the
// encrypt side, so concurrent callers of the Send* methods never interleave
// partial frames; the background goroutine started by DialLoginLink is the
// only reader.
type LoginLink struct {
	conn  net.Conn
	crypt *crypt.LinkCrypt
	log   *logrus.Logger
	done  chan struct{}

	// frames reuses one payload buffer across inbound frames; it belongs to
	// the single reader goroutine.
	frames *wire.FrameReader

	sendMu sync.Mutex

	// ServerID and ServerName are the identity the login server confirmed
	// for this game server during the handshake.
	ServerID   byte
	ServerName string
}

// DialLoginLink connects to the login server at address and performs the
// full GS-LS link handshake: it reads InitLS, rejects a protocol revision
// mismatch, generates a fresh dynamic Blowfish key and sends it RSA-
// encrypted with the login server's public key, switches the link to that
// key, then registers with auth. It returns once the login server has
// accepted or refused the registration; on acceptance, a background
// goroutine starts dispatching further inbound messages to handlers until
// the connection closes.
func DialLoginLink(ctx context.Context, address string, auth LoginServerAuth, handlers LoginLinkHandlers, log *logrus.Logger) (*LoginLink, error) {
	if log == nil {
		log = logrus.StandardLogger()
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("dial login server: %w", err)
	}

	l := &LoginLink{conn: conn, crypt: crypt.NewLinkCrypt(), log: log, done: make(chan struct{}), frames: wire.NewFrameReader(conn)}
	if err := l.handshake(auth); err != nil {
		conn.Close()
		return nil, err
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				l.log.Errorf("login link reader panic: %v", r)
			}
		}()
		l.readLoop(handlers)
	}()
	return l, nil
}

// handshake drives the bootstrap-key InitLS/BlowFishKey exchange and the
// subsequent GameServerAuth registration, leaving the link on its dynamic
// key and ServerID/ServerName populated on success.
func (l *LoginLink) handshake(auth LoginServerAuth) error {
	payload, err := l.readFrame()
	if err != nil {
		return fmt.Errorf("read InitLS: %w", err)
	}
	if firstByte(payload) != link.OpcodeInitLS {
		return fmt.Errorf("first packet opcode = %#x, want InitLS", firstByte(payload))
	}
	revision, modulus, err := link.DecodeInitLS(payload)
	if err != nil {
		return err
	}
	if revision != link.ProtocolRevision {
		return fmt.Errorf("login server protocol revision %#x, want %#x", revision, link.ProtocolRevision)
	}
	// The modulus bytes may carry a leading 0x00 the login server added so a
	// signed reader never mistakes them for negative; SetBytes treats input
	// as unsigned magnitude, so a leading zero byte is harmless either way.
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: linkPublicExponent}

	dynamicKey, err := generateDynamicKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate dynamic link key: %w", err)
	}

	if err := l.send(link.EncodeBlowFishKey(pub, dynamicKey)); err != nil {
		return fmt.Errorf("send BlowFishKey: %w", err)
	}
	if err := l.crypt.SetKey(dynamicKey); err != nil {
		return fmt.Errorf("set dynamic link key: %w", err)
	}

	if err := l.send(link.EncodeGameServerAuth(link.GameServerAuth{
		DesiredID:         byte(auth.ServerID),
		AcceptAlternateID: auth.AcceptAlternateID,
		HostReserved:      auth.ReserveHost,
		HostName:          auth.HostName,
		Port:              auth.Port,
		MaxPlayers:        auth.MaxPlayers,
		HexID:             auth.HexID,
	})); err != nil {
		return fmt.Errorf("send GameServerAuth: %w", err)
	}

	payload, err = l.readFrame()
	if err != nil {
		return fmt.Errorf("read registration result: %w", err)
	}
	switch firstByte(payload) {
	case link.OpcodeAuthResponse:
		id, name, err := link.DecodeAuthResponse(payload)
		if err != nil {
			return err
		}
		l.ServerID, l.ServerName = id, name
		return nil
	case link.OpcodeLoginServerFail:
		reason, err := link.DecodeLoginServerFail(payload)
		if err != nil {
			return err
		}
		return fmt.Errorf("login server refused registration: %s", reason)
	default:
		return fmt.Errorf("registration result opcode = %#x, want AuthResponse or LoginServerFail", firstByte(payload))
	}
}

// readLoop dispatches inbound messages to handlers until the connection
// closes or a fatal protocol error occurs, then closes the connection and
// signals Done.
func (l *LoginLink) readLoop(handlers LoginLinkHandlers) {
	defer close(l.done)
	defer l.conn.Close()

	for {
		payload, err := l.readFrame()
		if err != nil {
			return
		}
		switch firstByte(payload) {
		case link.OpcodePlayerAuthResponse:
			account, ok, err := link.DecodePlayerAuthResponse(payload)
			if err != nil {
				l.log.Warnf("login link: %v", err)
				continue
			}
			if handlers.PlayerAuthResponse != nil {
				handlers.PlayerAuthResponse(account, ok)
			}
		case link.OpcodeKickPlayer:
			account, err := link.DecodeKickPlayer(payload)
			if err != nil {
				l.log.Warnf("login link: %v", err)
				continue
			}
			if handlers.KickPlayer != nil {
				handlers.KickPlayer(account)
			}
		case link.OpcodeLoginServerFail:
			if reason, err := link.DecodeLoginServerFail(payload); err == nil {
				l.log.Warnf("login server closed the link: %s", reason)
			}
			return
		default:
			l.log.Warnf("login link: unexpected opcode %#x", firstByte(payload))
			return
		}
	}
}

// readFrame returns the next decrypted inbound payload. It reuses a
// per-link buffer, so the payload is only valid until the next readFrame
// call; only the single reader goroutine may call it.
func (l *LoginLink) readFrame() ([]byte, error) {
	payload, err := l.frames.ReadFrame()
	if err != nil {
		return nil, err
	}
	if err := l.crypt.Decrypt(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (l *LoginLink) send(payload []byte) error {
	l.sendMu.Lock()
	defer l.sendMu.Unlock()
	return wire.WriteFrame(l.conn, l.crypt.Encrypt(payload))
}

func firstByte(payload []byte) byte {
	if len(payload) == 0 {
		return 0
	}
	return payload[0]
}

// SendPlayerAuthRequest asks the login server to validate req's session
// keys for a client entering this game server.
func (l *LoginLink) SendPlayerAuthRequest(req link.PlayerAuthRequest) error {
	return l.send(link.EncodePlayerAuthRequest(req))
}

// SendPlayerLogout reports that account just logged out of this game
// server.
func (l *LoginLink) SendPlayerLogout(account string) error {
	return l.send(link.EncodePlayerLogout(account))
}

// SendPlayerInGame reports accounts that just entered the game on this
// server.
func (l *LoginLink) SendPlayerInGame(accounts []string) error {
	return l.send(link.EncodePlayerInGame(accounts))
}

// SendAccessLevelChange asks the login server to change account's access
// level.
func (l *LoginLink) SendAccessLevelChange(account string, level int32) error {
	return l.send(link.EncodeChangeAccessLevel(link.ChangeAccessLevel{Level: level, Account: account}))
}

// SendServerStatus pushes updated status attributes about this game
// server; status's nil fields leave that attribute unchanged.
func (l *LoginLink) SendServerStatus(status link.ServerStatus) error {
	return l.send(link.EncodeServerStatus(status))
}

// Done returns a channel that is closed once the link's read loop has
// ended, whether from the connection closing or a fatal protocol error.
func (l *LoginLink) Done() <-chan struct{} {
	return l.done
}

// Close closes the underlying connection, ending the link's read loop.
func (l *LoginLink) Close() error {
	return l.conn.Close()
}

// Maintain keeps this game server linked to the login server at address for
// as long as ctx is not canceled: it dials and performs the handshake,
// passes the established LoginLink to onLink, then blocks until the link
// ends and dials again after retryDelay. A failed dial or handshake is
// logged and retried the same way. onLink may be nil.
func Maintain(ctx context.Context, address string, auth LoginServerAuth, handlers LoginLinkHandlers, retryDelay time.Duration, onLink func(*LoginLink), log *logrus.Logger) {
	if log == nil {
		log = logrus.StandardLogger()
	}

	for ctx.Err() == nil {
		l, err := DialLoginLink(ctx, address, auth, handlers, log)
		if err != nil {
			log.Errorf("login link: %v", err)
		} else {
			if onLink != nil {
				onLink(l)
			}
			select {
			case <-ctx.Done():
				l.Close()
			case <-l.Done():
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(retryDelay):
		}
	}
}

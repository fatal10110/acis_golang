package botclient

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	commoncrypt "github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	logincrypt "github.com/fatal10110/acis_golang/internal/loginserver/crypt"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

// Credential block layout of RequestAuthLogin: the name and password sit at
// fixed offsets of one 128-byte RSA block.
const (
	authBlockSize   = 128
	authNameOffset  = 0x5e
	authNameMax     = 14
	authPassOffset  = 0x6c
	authPassMax     = 16
	initPayloadSize = 1 + 4 + 4 + 128 + 16 + 16 + 1
)

// SessionKeys are the login and play key pairs the login server hands out;
// the game server checks them against the login server on AuthLogin.
type SessionKeys struct {
	LoginKey1, LoginKey2 int32
	PlayKey1, PlayKey2   int32
}

// ServerEntry is one row of the login server's ServerList.
type ServerEntry struct {
	ID     byte
	Addr   string
	Online bool
}

// LoginResult is what a completed login-server session yields.
type LoginResult struct {
	Keys SessionKeys
	// GameAddr is the address the chosen server advertised.
	GameAddr string
	// Connected is when the TCP connection and Init completed.
	Connected time.Time
}

// LoginError reports a login-server rejection: the reply opcode and its reason.
type LoginError struct {
	Packet string
	Reason int32
}

func (e *LoginError) Error() string {
	return fmt.Sprintf("login server replied %s (reason %d)", e.Packet, e.Reason)
}

// loginConn is the client side of one login-server connection.
type loginConn struct {
	conn    net.Conn
	frames  *wire.FrameReader
	crypt   *logincrypt.LoginCrypt
	timeout time.Duration
}

func (c *loginConn) send(payload []byte) error {
	if err := c.conn.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
		return err
	}
	return wire.WriteFrame(c.conn, c.crypt.Encrypt(payload))
}

func (c *loginConn) read() ([]byte, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, err
	}
	payload, err := c.frames.ReadFrame()
	if err != nil {
		return nil, err
	}
	if err := c.crypt.Decrypt(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// Login runs one login-server session for account: Init, GameGuard,
// credentials, the server list and server selection. With no ServerList
// entry for serverID it fails. Each read and write waits at most timeout.
func Login(ctx context.Context, addr, account, password string, serverID byte, timeout time.Duration) (LoginResult, error) {
	var res LoginResult
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return res, fmt.Errorf("dial login server: %w", err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()

	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return res, err
	}
	frames := wire.NewFrameReader(conn)
	first, err := frames.ReadFrame()
	if err != nil {
		return res, fmt.Errorf("read Init: %w", err)
	}
	init, err := decodeInit(first)
	if err != nil {
		return res, err
	}
	cr, err := logincrypt.NewClientLoginCrypt(init.blowfishKey)
	if err != nil {
		return res, err
	}
	res.Connected = time.Now()
	c := &loginConn{conn: conn, frames: frames, crypt: cr, timeout: timeout}

	if err := c.send(encodeAuthGameGuard(init.sessionID)); err != nil {
		return res, fmt.Errorf("send AuthGameGuard: %w", err)
	}
	reply, err := c.read()
	if err != nil {
		return res, fmt.Errorf("read GGAuth: %w", err)
	}
	if err := expectLoginReply(reply, serverpackets.OpcodeGGAuth, "GGAuth"); err != nil {
		return res, err
	}

	if err := c.send(encodeRequestAuthLogin(init.publicKey, account, password)); err != nil {
		return res, fmt.Errorf("send RequestAuthLogin: %w", err)
	}
	if reply, err = c.read(); err != nil {
		return res, fmt.Errorf("read login reply: %w", err)
	}
	// With ShowLicence off the server answers the credentials with the
	// ServerList itself and does not check the session keys afterwards.
	if len(reply) > 0 && reply[0] == serverpackets.OpcodeLoginOk {
		res.Keys.LoginKey1, res.Keys.LoginKey2 = decodeLoginOk(reply)
		if err := c.send(encodeRequestServerList(res.Keys.LoginKey1, res.Keys.LoginKey2)); err != nil {
			return res, fmt.Errorf("send RequestServerList: %w", err)
		}
		if reply, err = c.read(); err != nil {
			return res, fmt.Errorf("read ServerList: %w", err)
		}
	}
	if err := expectLoginReply(reply, serverpackets.OpcodeServerList, "ServerList"); err != nil {
		return res, err
	}
	servers, err := decodeServerList(reply)
	if err != nil {
		return res, err
	}
	for _, s := range servers {
		if s.ID == serverID {
			res.GameAddr = s.Addr
		}
	}
	if res.GameAddr == "" {
		return res, fmt.Errorf("server %d is not in the ServerList", serverID)
	}

	if err := c.send(encodeRequestServerLogin(res.Keys.LoginKey1, res.Keys.LoginKey2, serverID)); err != nil {
		return res, fmt.Errorf("send RequestServerLogin: %w", err)
	}
	if reply, err = c.read(); err != nil {
		return res, fmt.Errorf("read PlayOk: %w", err)
	}
	if err := expectLoginReply(reply, serverpackets.OpcodePlayOk, "PlayOk"); err != nil {
		return res, err
	}
	res.Keys.PlayKey1, res.Keys.PlayKey2 = decodePlayOk(reply)
	return res, nil
}

// expectLoginReply maps a LoginFail, PlayFail or unexpected reply to an error.
func expectLoginReply(reply []byte, opcode byte, name string) error {
	if len(reply) == 0 {
		return errors.New("empty login reply")
	}
	r := wire.NewReader(reply[1:])
	switch reply[0] {
	case opcode:
		return nil
	case serverpackets.OpcodeLoginFail:
		return &LoginError{Packet: "LoginFail", Reason: r.ReadInt32()}
	case serverpackets.OpcodePlayFail:
		return &LoginError{Packet: "PlayFail", Reason: int32(r.ReadUint8())}
	case serverpackets.OpcodeAccountKicked:
		return &LoginError{Packet: "AccountKicked", Reason: r.ReadInt32()}
	}
	return fmt.Errorf("login reply opcode %#x, want %s (%#x)", reply[0], name, opcode)
}

type initPacket struct {
	sessionID   int32
	publicKey   *rsa.PublicKey
	blowfishKey []byte
}

// decodeInit decrypts the first login packet in place and parses it.
func decodeInit(frame []byte) (initPacket, error) {
	var p initPacket
	if err := logincrypt.DecryptFirst(frame); err != nil {
		return p, err
	}
	if len(frame) < initPayloadSize || frame[0] != serverpackets.OpcodeInit {
		return p, fmt.Errorf("first login packet is not Init (%d bytes, opcode %#x)", len(frame), frame[0])
	}
	r := wire.NewReader(frame[1:])
	p.sessionID = r.ReadInt32()
	_ = r.ReadInt32() // protocol revision
	pub, err := commoncrypt.UnscrambleModulus(r.ReadBytes(128))
	if err != nil {
		return p, err
	}
	p.publicKey = pub
	_ = r.ReadBytes(16) // GameGuard placeholder
	p.blowfishKey = append([]byte(nil), r.ReadBytes(16)...)
	return p, r.Err()
}

func decodeLoginOk(reply []byte) (key1, key2 int32) {
	r := wire.NewReader(reply[1:])
	return r.ReadInt32(), r.ReadInt32()
}

func decodePlayOk(reply []byte) (key1, key2 int32) {
	r := wire.NewReader(reply[1:])
	return r.ReadInt32(), r.ReadInt32()
}

func decodeServerList(reply []byte) ([]ServerEntry, error) {
	r := wire.NewReader(reply[1:])
	n := int(r.ReadUint8())
	_ = r.ReadUint8() // last server
	servers := make([]ServerEntry, 0, n)
	for range n {
		var s ServerEntry
		s.ID = r.ReadUint8()
		ip := net.IP(r.ReadBytes(4))
		port := r.ReadInt32()
		s.Addr = net.JoinHostPort(ip.String(), strconv.Itoa(int(port)))
		_ = r.ReadUint8()  // age limit
		_ = r.ReadUint8()  // PvP
		_ = r.ReadUint16() // current players
		_ = r.ReadUint16() // max players
		s.Online = r.ReadUint8() != 0
		_ = r.ReadInt32() // server type bits
		_ = r.ReadUint8() // brackets
		servers = append(servers, s)
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("decode ServerList: %w", err)
	}
	return servers, nil
}

func encodeAuthGameGuard(sessionID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAuthGameGuard)
	w.WriteInt32(sessionID)
	for range 4 {
		w.WriteInt32(0)
	}
	return w.Bytes()
}

// encodeRequestAuthLogin RSA-encrypts (no padding) the credential block
// with the session's public key.
func encodeRequestAuthLogin(pub *rsa.PublicKey, account, password string) []byte {
	var block [authBlockSize]byte
	copy(block[authNameOffset:authNameOffset+authNameMax], account)
	copy(block[authPassOffset:authPassOffset+authPassMax], password)
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAuthLogin)
	w.WriteBytes(commoncrypt.EncryptDynamicKey(pub, block[:]))
	return w.Bytes()
}

func encodeRequestServerList(key1, key2 int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestServerList)
	w.WriteInt32(key1)
	w.WriteInt32(key2)
	return w.Bytes()
}

func encodeRequestServerLogin(key1, key2 int32, serverID byte) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestServerLogin)
	w.WriteInt32(key1)
	w.WriteInt32(key2)
	w.WriteUint8(serverID)
	return w.Bytes()
}

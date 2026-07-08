package loginserver

import (
	"bytes"
	"context"
	"crypto/rsa"
	"net"

	"github.com/sirupsen/logrus"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
)

// GameServerLink accepts and drives connections from game servers over the
// GS<->LS link protocol: the handshake, registration, and the runtime
// status/session messages a linked game server exchanges with this login
// server.
type GameServerLink struct {
	servers         *manager.ServerRegistry
	names           *manager.ServerNames
	keys            *manager.RSAKeyPool
	sessions        *manager.SessionStore
	bans            *manager.IPBanList
	accounts        *sql.AccountStore
	registrations   *sql.GameServerStore
	allowNewServers bool
	log             *logrus.Logger
}

// NewGameServerLink builds a GameServerLink from its collaborators.
// allowNewServers mirrors the AcceptNewGameServer config flag.
func NewGameServerLink(
	servers *manager.ServerRegistry,
	names *manager.ServerNames,
	keys *manager.RSAKeyPool,
	sessions *manager.SessionStore,
	bans *manager.IPBanList,
	accounts *sql.AccountStore,
	registrations *sql.GameServerStore,
	allowNewServers bool,
	log *logrus.Logger,
) *GameServerLink {
	if log == nil {
		log = logrus.StandardLogger()
	}
	return &GameServerLink{
		servers:         servers,
		names:           names,
		keys:            keys,
		sessions:        sessions,
		bans:            bans,
		accounts:        accounts,
		registrations:   registrations,
		allowNewServers: allowNewServers,
		log:             log,
	}
}

// Serve accepts game-server connections on ln until ctx is canceled or
// accepting fails. Each connection is handled on its own goroutine. The
// caller owns ln: Serve closes it on ctx cancellation but does not create
// it, so tests can bind an ephemeral port and callers can control the
// listen address/network.
func (l *GameServerLink) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				l.log.Errorf("gameserver link shutdown watcher panic: %v", r)
			}
		}()
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					l.log.Errorf("gameserver link connection handler panic: %v", r)
				}
			}()
			l.handleConnection(ctx, conn)
		}()
	}
}

// gameServerConn is one game server's link connection. It is owned
// entirely by the goroutine running handleConnection: nothing else writes
// to conn or advances crypt.
type gameServerConn struct {
	conn     net.Conn
	remoteIP net.IP
	crypt    *crypt.LinkCrypt
	key      *rsa.PrivateKey
	id       int
	authed   bool
}

func (c *gameServerConn) send(payload []byte) error {
	return wire.WriteFrame(c.conn, c.crypt.Encrypt(payload))
}

// forceClose sends a LoginServerFail with reason; the caller closes conn.
func (c *gameServerConn) forceClose(reason link.LoginServerFailReason) {
	_ = c.send(link.EncodeLoginServerFail(reason))
}

func (l *GameServerLink) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	c := &gameServerConn{
		conn:     conn,
		remoteIP: remoteIP(conn),
		crypt:    crypt.NewLinkCrypt(),
		key:      l.keys.Random(),
	}
	defer func() {
		if c.authed {
			l.servers.MarkOffline(c.id)
			l.log.Infof("gameserver [%d] disconnected from the link", c.id)
		}
	}()

	if l.bans.IsBanned(c.remoteIP) {
		l.log.Infof("banned gameserver with ip %s tried to link", c.remoteIP)
		c.forceClose(link.ReasonIPBanned)
		return
	}

	if err := c.send(link.EncodeInitLS(crypt.ModulusBytes(&c.key.PublicKey))); err != nil {
		return
	}

	for {
		payload, err := wire.ReadFrame(conn)
		if err != nil {
			return
		}
		if err := c.crypt.Decrypt(payload); err != nil {
			l.log.Warnf("gameserver link %s: %v", c.remoteIP, err)
			return
		}
		if len(payload) == 0 {
			return
		}

		switch payload[0] {
		case link.OpcodeBlowFishKey:
			l.onBlowFishKey(c, payload)
		case link.OpcodeGameServerAuth:
			if !l.onGameServerAuth(ctx, c, payload) {
				return
			}
		default:
			if !c.authed {
				c.forceClose(link.ReasonNotAuthed)
				return
			}
			switch payload[0] {
			case link.OpcodePlayerInGame:
				l.onPlayerInGame(c, payload)
			case link.OpcodePlayerLogout:
				l.onPlayerLogout(c, payload)
			case link.OpcodeChangeAccessLevel:
				l.onChangeAccessLevel(ctx, c, payload)
			case link.OpcodePlayerAuthRequest:
				l.onPlayerAuthRequest(c, payload)
			case link.OpcodeServerStatus:
				l.onServerStatus(c, payload)
			default:
				c.forceClose(link.ReasonNotAuthed)
				return
			}
		}
	}
}

func (l *GameServerLink) onBlowFishKey(c *gameServerConn, payload []byte) {
	key, err := link.DecodeBlowFishKey(payload, c.key)
	if err != nil {
		l.log.Warnf("gameserver link %s: %v", c.remoteIP, err)
		return
	}
	if err := c.crypt.SetKey(key); err != nil {
		l.log.Warnf("gameserver link %s: %v", c.remoteIP, err)
	}
}

// onGameServerAuth handles a registration/re-authentication request: reuse
// a matching entry, allocate an alternate id when the desired one is taken
// by a different key and that is permitted, or create a fresh entry.
// Returns false if the connection must close.
func (l *GameServerLink) onGameServerAuth(ctx context.Context, c *gameServerConn, payload []byte) bool {
	auth, err := link.DecodeGameServerAuth(payload)
	if err != nil {
		l.log.Warnf("gameserver link %s: %v", c.remoteIP, err)
		return false
	}

	id := int(auth.DesiredID)
	entry, exists := l.servers.Get(id)

	switch {
	case exists && bytes.Equal(entry.HexID, auth.HexID):
		if entry.Authed {
			c.forceClose(link.ReasonAlreadyLoggedIn)
			return false
		}

	case exists:
		if !l.allowNewServers || !auth.AcceptAlternateID {
			c.forceClose(link.ReasonWrongHexID)
			return false
		}
		created, ok := l.servers.RegisterFirst(l.names.IDs(), auth.HexID)
		if !ok {
			c.forceClose(link.ReasonNoFreeID)
			return false
		}
		id = created.ID
		l.persistRegistration(ctx, id, auth.HexID)

	default:
		if !l.allowNewServers {
			c.forceClose(link.ReasonWrongHexID)
			return false
		}
		if _, ok := l.servers.Register(id, auth.HexID); !ok {
			c.forceClose(link.ReasonIDReserved)
			return false
		}
		l.persistRegistration(ctx, id, auth.HexID)
	}

	host := auth.HostName
	if host != "*" {
		if resolved, err := net.LookupHost(host); err == nil && len(resolved) > 0 {
			host = resolved[0]
		} else {
			host = c.remoteIP.String()
		}
	} else {
		host = c.remoteIP.String()
	}

	l.servers.MarkOnline(id, host, auth.Port, auth.MaxPlayers)
	c.id = id
	c.authed = true

	name, _ := l.names.Name(id)
	if err := c.send(link.EncodeAuthResponse(byte(id), name)); err != nil {
		return false
	}
	return true
}

func (l *GameServerLink) persistRegistration(ctx context.Context, id int, hexID []byte) {
	if err := l.registrations.CreateGameServer(ctx, model.NewGameServer(id, hexID, "")); err != nil {
		l.log.Errorf("persist gameserver %d registration: %v", id, err)
	}
}

func (l *GameServerLink) onPlayerInGame(c *gameServerConn, payload []byte) {
	accounts, err := link.DecodePlayerInGame(payload)
	if err != nil {
		l.log.Warnf("gameserver link %s: %v", c.remoteIP, err)
		return
	}
	for _, account := range accounts {
		l.servers.AddOnlineAccount(c.id, account)
	}
}

func (l *GameServerLink) onPlayerLogout(c *gameServerConn, payload []byte) {
	account, err := link.DecodePlayerLogout(payload)
	if err != nil {
		l.log.Warnf("gameserver link %s: %v", c.remoteIP, err)
		return
	}
	l.servers.RemoveOnlineAccount(c.id, account)
}

func (l *GameServerLink) onChangeAccessLevel(ctx context.Context, c *gameServerConn, payload []byte) {
	cal, err := link.DecodeChangeAccessLevel(payload)
	if err != nil {
		l.log.Warnf("gameserver link %s: %v", c.remoteIP, err)
		return
	}
	if err := l.accounts.SetAccessLevel(ctx, cal.Account, int(cal.Level)); err != nil {
		l.log.Errorf("change access level for %s: %v", cal.Account, err)
	}
}

// onPlayerAuthRequest validates a client's session keys, presented by the
// game server the client is entering, against the session this login
// server issued. Nothing currently calls SessionStore.Put, since the
// client-facing login flow that issues sessions is not built yet — until
// it is, every request here correctly fails validation for lack of a
// stored session.
func (l *GameServerLink) onPlayerAuthRequest(c *gameServerConn, payload []byte) {
	req, err := link.DecodePlayerAuthRequest(payload)
	if err != nil {
		l.log.Warnf("gameserver link %s: %v", c.remoteIP, err)
		return
	}

	key, ok := l.sessions.Get(req.Account)
	valid := ok && key == req.SessionKey
	if valid {
		l.sessions.Delete(req.Account)
	}

	if err := c.send(link.EncodePlayerAuthResponse(req.Account, valid)); err != nil {
		return
	}
}

func (l *GameServerLink) onServerStatus(c *gameServerConn, payload []byte) {
	status, err := link.DecodeServerStatus(payload)
	if err != nil {
		l.log.Warnf("gameserver link %s: %v", c.remoteIP, err)
		return
	}
	l.servers.ApplyStatus(c.id, status)
}

func remoteIP(conn net.Conn) net.IP {
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		return addr.IP
	}
	host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}

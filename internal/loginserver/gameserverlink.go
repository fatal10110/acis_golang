package loginserver

import (
	"bytes"
	"context"
	"crypto/rsa"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/netutil"
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
	registrations   registrationStore
	allowNewServers bool
	flood           *netutil.FloodGuard
	roster          *LinkRoster
	log             zerolog.Logger
}

type registrationStore interface {
	CreateGameServer(ctx context.Context, server model.GameServer) error
}

// NewGameServerLink builds a GameServerLink from its collaborators.
// allowNewServers mirrors the AcceptNewGameServer config flag; flood, when
// non-nil, throttles connections per remote IP before they are handled;
// roster records each authed connection so other components can deliver a
// message to that server's link.
func NewGameServerLink(
	servers *manager.ServerRegistry,
	names *manager.ServerNames,
	keys *manager.RSAKeyPool,
	sessions *manager.SessionStore,
	bans *manager.IPBanList,
	accounts *sql.AccountStore,
	registrations registrationStore,
	allowNewServers bool,
	flood *netutil.FloodGuard,
	roster *LinkRoster,
	log zerolog.Logger,
) *GameServerLink {
	return &GameServerLink{
		servers:         servers,
		names:           names,
		keys:            keys,
		sessions:        sessions,
		bans:            bans,
		accounts:        accounts,
		registrations:   registrations,
		allowNewServers: allowNewServers,
		flood:           flood,
		roster:          roster,
		log:             log,
	}
}

// Serve accepts game-server connections on ln until ctx is canceled or
// accepting fails. Each connection is handled on its own goroutine. The
// caller owns ln: Serve closes it on ctx cancellation but does not create
// it, so tests can bind an ephemeral port and callers can control the
// listen address/network.
func (l *GameServerLink) Serve(ctx context.Context, ln net.Listener) error {
	return netutil.AcceptLoop(ctx, ln, func(conn net.Conn) {
		if l.flood != nil {
			ip := remoteIP(conn)
			if !l.flood.CanConnect(ip.String(), time.Now()) {
				l.log.Info().Str("ip", ip.String()).Msg("flood-protected link connection dropped")
				conn.Close()
				return
			}
			defer l.flood.Release(ip.String())
		}
		l.handleConnection(ctx, conn)
	}, l.log)
}

// gameServerConn is one game server's link connection. Its handler
// goroutine owns the protocol flow; the roster may additionally deliver a
// message from outside, so sends are serialized by sendMu.
type gameServerConn struct {
	conn     net.Conn
	remoteIP net.IP
	crypt    *crypt.LinkCrypt
	key      *rsa.PrivateKey
	id       int
	authed   bool

	sendMu sync.Mutex
}

func (c *gameServerConn) send(payload []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return wire.WriteFrame(c.conn, c.crypt.Encrypt(payload))
}

// LinkRoster tracks each linked game server's live connection, so a
// component outside the link's own read loop (ClientLink evicting an account
// that re-authenticated) can deliver a message to that server. Build one
// with NewLinkRoster and share it between GameServerLink and ClientLink at
// composition time.
//
// mu guards conns.
type LinkRoster struct {
	mu    sync.RWMutex
	conns map[int]*gameServerConn
}

// NewLinkRoster returns an empty LinkRoster.
func NewLinkRoster() *LinkRoster {
	return &LinkRoster{conns: make(map[int]*gameServerConn)}
}

func (r *LinkRoster) add(id int, c *gameServerConn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns[id] = c
}

// remove drops id's entry only if it still points at c, so a replacement
// connection for the same server is never removed by the old one.
func (r *LinkRoster) remove(id int, c *gameServerConn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conns[id] == c {
		delete(r.conns, id)
	}
}

// kick asks the game server linked at id to disconnect account and reports
// whether the request was delivered.
func (r *LinkRoster) kick(id int, account string) bool {
	r.mu.RLock()
	c := r.conns[id]
	r.mu.RUnlock()
	if c == nil {
		return false
	}
	return c.send(link.EncodeKickPlayer(account)) == nil
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
			if l.roster != nil {
				l.roster.remove(c.id, c)
			}
			l.servers.MarkOffline(c.id)
			l.log.Info().Int("server_id", c.id).Msg("gameserver disconnected from the link")
		}
	}()

	if l.bans.IsBanned(c.remoteIP) {
		l.log.Info().Str("ip", c.remoteIP.String()).Msg("banned gameserver tried to link")
		c.forceClose(link.ReasonIPBanned)
		return
	}

	if err := c.send(link.EncodeInitLS(crypt.ModulusBytes(&c.key.PublicKey))); err != nil {
		return
	}

	// frames reuses one payload buffer across this connection's inbound
	// frames; every payload is decoded within its loop iteration.
	frames := wire.NewFrameReader(conn)
	for {
		payload, err := frames.ReadFrame()
		if err != nil {
			return
		}
		if err := c.crypt.Decrypt(payload); err != nil {
			l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("gameserver link")
			return
		}
		if len(payload) == 0 {
			return
		}

		switch payload[0] {
		case link.OpcodeBlowFishKey:
			if !l.onBlowFishKey(c, payload) {
				return
			}
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

// onBlowFishKey installs the dynamic Blowfish key the game server presents.
// A key that cannot be decrypted or installed leaves the link unusable, so
// the connection is dropped. It reports false when the connection must
// close.
func (l *GameServerLink) onBlowFishKey(c *gameServerConn, payload []byte) bool {
	key, err := link.DecodeBlowFishKey(payload, c.key)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("gameserver link: drop connection on undecryptable blowfish key")
		return false
	}
	if err := c.crypt.SetKey(key); err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("gameserver link: drop connection on invalid blowfish key")
		return false
	}
	return true
}

// onGameServerAuth handles a registration/re-authentication request: reuse
// a matching entry, allocate an alternate id when the desired one is taken
// by a different key and that is permitted, or create a fresh entry.
// Returns false if the connection must close.
func (l *GameServerLink) onGameServerAuth(ctx context.Context, c *gameServerConn, payload []byte) bool {
	auth, err := link.DecodeGameServerAuth(payload)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("gameserver link")
		return false
	}

	id := int(auth.DesiredID)
	entry, exists := l.servers.Get(id)
	persist := false
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
		persist = true

	default:
		if !l.allowNewServers {
			c.forceClose(link.ReasonWrongHexID)
			return false
		}
		if _, ok := l.servers.Register(id, auth.HexID); !ok {
			c.forceClose(link.ReasonIDReserved)
			return false
		}
		persist = true
	}

	if persist {
		l.persistRegistration(ctx, id, auth.HexID, host)
	}

	if _, ok := l.servers.MarkOnline(id, host, c.remoteIP, auth.Port, auth.MaxPlayers); !ok {
		c.forceClose(link.ReasonAlreadyLoggedIn)
		return false
	}
	c.id = id
	c.authed = true
	if l.roster != nil {
		l.roster.add(id, c)
	}

	name, _ := l.names.Name(id)
	if err := c.send(link.EncodeAuthResponse(byte(id), name)); err != nil {
		return false
	}
	return true
}

func (l *GameServerLink) persistRegistration(ctx context.Context, id int, hexID []byte, host string) {
	if l.registrations == nil {
		l.log.Error().Int("server_id", id).Msg("persist gameserver registration")
		return
	}
	if err := l.registrations.CreateGameServer(ctx, model.NewGameServer(id, hexID, host)); err != nil {
		l.log.Error().Int("server_id", id).Err(err).Msg("persist gameserver registration")
	}
}

func (l *GameServerLink) onPlayerInGame(c *gameServerConn, payload []byte) {
	accounts, err := link.DecodePlayerInGame(payload)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("gameserver link")
		return
	}
	for _, account := range accounts {
		l.servers.AddOnlineAccount(c.id, account)
	}
}

func (l *GameServerLink) onPlayerLogout(c *gameServerConn, payload []byte) {
	account, err := link.DecodePlayerLogout(payload)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("gameserver link")
		return
	}
	l.servers.RemoveOnlineAccount(c.id, account)
}

func (l *GameServerLink) onChangeAccessLevel(ctx context.Context, c *gameServerConn, payload []byte) {
	cal, err := link.DecodeChangeAccessLevel(payload)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("gameserver link")
		return
	}
	if err := l.accounts.SetAccessLevel(ctx, cal.Account, int(cal.Level)); err != nil {
		l.log.Error().Str("account", cal.Account).Err(err).Msg("change access level")
	}
}

// onPlayerAuthRequest validates a client's session keys, presented by the
// game server the client is entering, against the session this login
// server issued (ClientLink.onRequestAuthLogin/onRequestServerLogin).
func (l *GameServerLink) onPlayerAuthRequest(c *gameServerConn, payload []byte) {
	req, err := link.DecodePlayerAuthRequest(payload)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("gameserver link")
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
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("gameserver link")
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

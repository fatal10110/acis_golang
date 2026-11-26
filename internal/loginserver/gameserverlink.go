package loginserver

import (
	"bytes"
	"context"
	"crypto/rsa"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/netutil"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
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
	accounts        accessLevelStore
	registrations   registrationStore
	allowNewServers bool
	flood           *netutil.FloodGuard
	roster          *LinkRoster
	log             zerolog.Logger

	// lookupHost resolves a registering server's advertised host name.
	lookupHost func(string) ([]string, error)
}

type registrationStore interface {
	CreateGameServer(ctx context.Context, server model.GameServer) error
}

type accessLevelStore interface {
	SetAccessLevel(ctx context.Context, login string, level int) error
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
	accounts accessLevelStore,
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
		lookupHost:      net.LookupHost,
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
	writes   *linkWriter

	sendMu sync.Mutex
}

// linkWriteQueue is how many database writes one link may have queued
// before its read loop waits for a free slot.
const linkWriteQueue = 64

// linkWriter runs one link's database writes on a goroutine of its own, in
// arrival order and each bounded by loginDBTimeout, so a stalled database
// never holds up the requests the read loop answers. The read loop owns
// enqueue and close; the writer goroutine owns the calls and exits once
// close has been called and the queue is drained.
type linkWriter struct {
	ctx  context.Context
	jobs chan func(context.Context)
	done chan struct{}
}

func startLinkWriter(ctx context.Context) *linkWriter {
	w := &linkWriter{ctx: ctx, jobs: make(chan func(context.Context), linkWriteQueue), done: make(chan struct{})}
	go func() {
		defer close(w.done)
		for job := range w.jobs {
			jobCtx, cancel := context.WithTimeout(ctx, loginDBTimeout)
			job(jobCtx)
			cancel()
		}
	}()
	return w
}

// enqueue queues job. With the queue full it waits for a slot, which the
// per-write timeout bounds, unless the link is shutting down.
func (w *linkWriter) enqueue(job func(context.Context)) {
	select {
	case w.jobs <- job:
	case <-w.ctx.Done():
	}
}

// close stops the writer and waits for the writes already queued.
func (w *linkWriter) close() {
	close(w.jobs)
	<-w.done
}

func (c *gameServerConn) send(payload []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if err := setWriteDeadline(c.conn); err != nil {
		return err
	}
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
	writes := startLinkWriter(ctx)
	// Deferred before the close, so queued writes finish after the
	// connection is already closed rather than holding it open.
	defer writes.close()
	defer conn.Close()

	c := &gameServerConn{
		conn:     conn,
		remoteIP: remoteIP(conn),
		crypt:    crypt.NewLinkCrypt(),
		key:      l.keys.Random(),
		writes:   writes,
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
		deadline := time.Now().Add(connectionHandshakeTimeout)
		if c.authed {
			deadline = time.Time{}
		}
		if err := conn.SetReadDeadline(deadline); err != nil {
			return
		}
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
				l.onChangeAccessLevel(c, payload)
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
// the connection is dropped, as is a key presented once the server has
// registered: the key is set once, before registration, and a registered
// link never switches keys under the roster's sends. It reports false when
// the connection must close.
func (l *GameServerLink) onBlowFishKey(c *gameServerConn, payload []byte) bool {
	if c.authed {
		l.log.Warn().Str("ip", c.remoteIP.String()).Int("server_id", c.id).Msg("gameserver link: drop connection on blowfish key after registration")
		return false
	}
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

	// Only an accepted registration resolves its advertised host.
	host := auth.HostName
	if host != "*" {
		if resolved, err := l.lookupHost(host); err == nil && len(resolved) > 0 {
			host = advertisedAddress(resolved)
		} else {
			l.log.Error().Str("host", host).Err(err).Msg("gameserver link: couldn't resolve hostname")
			host = c.remoteIP.String()
		}
	} else {
		host = c.remoteIP.String()
	}

	// A registration that cannot be stored is refused rather than accepted
	// in memory only, where it would be lost on the next restart (#5048).
	// No LoginServerFail reason fits a storage fault, so the link is closed
	// without an answer.
	if persist {
		if err := l.persistRegistration(ctx, id, auth.HexID, host); err != nil {
			l.servers.Unregister(id, auth.HexID)
			l.log.Error().Int("server_id", id).Str("ip", c.remoteIP.String()).Err(err).Msg("gameserver link: refuse registration that could not be stored")
			return false
		}
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

// advertisedAddress picks the address a registering server is advertised on
// from a non-empty resolver result: the first IPv4 address, because the
// server list carries four address bytes, else the first address.
func advertisedAddress(resolved []string) string {
	for _, addr := range resolved {
		if ip4 := net.ParseIP(addr).To4(); ip4 != nil {
			return ip4.String()
		}
	}
	return resolved[0]
}

func (l *GameServerLink) persistRegistration(ctx context.Context, id int, hexID []byte, host string) error {
	if l.registrations == nil {
		return errors.New("no registration store")
	}
	// Runs on the read loop: the server awaits its AuthResponse before it
	// may send anything else, so no request can queue behind this write,
	// and the timeout bounds how long the answer waits on the database.
	ctx, cancel := context.WithTimeout(ctx, loginDBTimeout)
	defer cancel()
	return l.registrations.CreateGameServer(ctx, model.NewGameServer(id, hexID, host))
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

// onChangeAccessLevel queues the access-level write on the link's writer, so
// the read loop goes on answering requests while the database works.
func (l *GameServerLink) onChangeAccessLevel(c *gameServerConn, payload []byte) {
	cal, err := link.DecodeChangeAccessLevel(payload)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("gameserver link")
		return
	}
	// Only an account this server admitted with valid session keys, and not
	// since logged out or admitted elsewhere, may have its access level
	// changed by it; a linked server cannot promote or ban any other account,
	// whatever names it reports online (#4655).
	if !l.servers.AdmittedOn(c.id, cal.Account) {
		l.log.Warn().Int("server_id", c.id).Str("account", cal.Account).Int32("level", cal.Level).Msg("change access level: ignored, account not online on this server")
		return
	}
	if l.accounts == nil {
		l.log.Error().Str("account", cal.Account).Msg("change access level: no account store")
		return
	}
	c.writes.enqueue(func(ctx context.Context) {
		if err := l.accounts.SetAccessLevel(ctx, cal.Account, int(cal.Level)); err != nil {
			l.log.Error().Str("account", cal.Account).Err(err).Msg("change access level")
		}
	})
}

// onPlayerAuthRequest validates a client's session keys, presented by the
// game server the client is entering, against the session this login
// server issued (ClientLink.onRequestAuthLogin).
func (l *GameServerLink) onPlayerAuthRequest(c *gameServerConn, payload []byte) {
	req, err := link.DecodePlayerAuthRequest(payload)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("gameserver link")
		return
	}

	// Validation consumes the session atomically, so two linked servers
	// presenting the same keys cannot both be admitted, and only the server
	// the client chose may claim it (#4655).
	valid := l.sessions.Claim(req.Account, req.SessionKey, c.id)
	if valid {
		l.servers.AdmitAccount(c.id, req.Account)
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

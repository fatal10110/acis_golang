package loginserver

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	commoncrypt "github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/netutil"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
	logincrypt "github.com/fatal10110/acis_golang/internal/loginserver/crypt"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	loginsql "github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

const (
	// DefaultLoginTryBeforeBan is the default number of failed credential
	// attempts allowed from one IP before it is banned.
	DefaultLoginTryBeforeBan = 3
	// DefaultLoginBlockAfterBan is the default temporary IP ban duration.
	DefaultLoginBlockAfterBan = 10 * time.Minute
	// LoginTimeout is how long an authenticated client may hold its
	// session without joining a game server before the purge loop drops it.
	LoginTimeout = 60 * time.Second
)

// accountStore is the account persistence ClientLink needs. *sql.AccountStore
// satisfies it in production; tests substitute an in-memory fake so the
// login flow can be exercised without a database.
type accountStore interface {
	Account(ctx context.Context, login string) (model.Account, error)
	CreateAccount(ctx context.Context, login, hashedPassword string, createdAt time.Time) (model.Account, error)
	SetLastServer(ctx context.Context, login string, serverID int) error
	SetLastActive(ctx context.Context, login string, at time.Time) error
}

// ClientLink accepts and drives connections from Interlude game clients over
// the login protocol: the Init/crypto handshake, credential authentication,
// and the server-list/play-server selection that hands a client's session
// off to a game server (validated there via GameServerLink.PlayerAuthRequest).
type ClientLink struct {
	accounts           accountStore
	servers            *manager.ServerRegistry
	sessions           *manager.SessionStore
	bans               *manager.IPBanList
	autoCreateAccounts bool
	skipLicenceCheck   bool
	loginTryBeforeBan  int
	loginBlockAfterBan time.Duration
	log                zerolog.Logger

	// failedMu guards failedAttempts.
	failedMu       sync.Mutex
	failedAttempts map[string]int

	// loginTimeout is how long an authed connection may outstay its
	// welcome before the purge loop closes it; zero disables purging.
	loginTimeout time.Duration

	// purgeMu guards purgeable, the set of authenticated connections the
	// purge loop walks. Connections join on successful authentication and
	// leave when their handler finishes.
	purgeMu   sync.Mutex
	purgeable map[*clientConn]struct{}

	// newKeyPair, newSessionKey, and newSessionID supply each connection's
	// RSA key pair, dynamic Blowfish key, and Init session id; overridden in
	// tests for a deterministic handshake.
	newKeyPair    func() *commoncrypt.LoginKeyPair
	newSessionKey func() ([]byte, error)
	newSessionID  func() int32
}

// NewClientLink builds a ClientLink from its collaborators. autoCreateAccounts
// mirrors the AutoCreateAccounts config flag: an unrecognized login is
// registered on its first successful RequestAuthLogin rather than rejected.
// skipLicenceCheck is the inverse of the ShowLicence config flag: when
// ShowLicence is false, a successful login replies ServerList instead of
// LoginOk and the session-key pair on RequestServerLogin is not enforced.
func NewClientLink(
	accounts *loginsql.AccountStore,
	servers *manager.ServerRegistry,
	sessions *manager.SessionStore,
	bans *manager.IPBanList,
	keys *manager.LoginKeyPool,
	autoCreateAccounts bool,
	showLicence bool,
	loginTryBeforeBan int,
	loginBlockAfterBan time.Duration,
	log zerolog.Logger,
) *ClientLink {
	return &ClientLink{
		accounts:           accounts,
		servers:            servers,
		sessions:           sessions,
		bans:               bans,
		autoCreateAccounts: autoCreateAccounts,
		skipLicenceCheck:   !showLicence,
		loginTryBeforeBan:  loginTryBeforeBan,
		loginBlockAfterBan: loginBlockAfterBan,
		log:                log,
		failedAttempts:     make(map[string]int),
		loginTimeout:       LoginTimeout,
		purgeable:          make(map[*clientConn]struct{}),
		newKeyPair:         keys.Random,
		newSessionKey:      logincrypt.NewSessionKey,
		newSessionID:       rand.Int32,
	}
}

// Serve accepts login-client connections on ln until ctx is canceled or
// accepting fails. Each connection is handled on its own goroutine. A
// purge loop closes authenticated connections that outstay LoginTimeout
// without joining a game server. The caller owns ln: Serve closes it on
// ctx cancellation but does not create it, so tests can bind an ephemeral
// port.
func (l *ClientLink) Serve(ctx context.Context, ln net.Listener) error {
	go l.purgeLoop(ctx)
	return netutil.AcceptLoop(ctx, ln, func(conn net.Conn) {
		l.handleConnection(ctx, conn)
	}, l.log)
}

// purgeLoop sweeps the authenticated connections every half login timeout,
// dropping those whose connection outlived it, as the reference's purge
// task does.
func (l *ClientLink) purgeLoop(ctx context.Context) {
	if l.loginTimeout <= 0 {
		return
	}
	ticker := time.NewTicker(l.loginTimeout / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.purgeStale(time.Now())
		}
	}
}

// purgeStale closes every registered connection that connected more than
// loginTimeout ago, replying with a LoginFail first.
func (l *ClientLink) purgeStale(now time.Time) {
	l.purgeMu.Lock()
	var stale []*clientConn
	for c := range l.purgeable {
		if now.Sub(c.connectedAt) > l.loginTimeout {
			stale = append(stale, c)
		}
	}
	l.purgeMu.Unlock()

	for _, c := range stale {
		l.log.Info().Str("ip", c.remoteIP.String()).Str("account", c.account).Msg("purging login client stuck past the login timeout")
		c.kick()
	}
}

// registerPurgeable adds c to the purge set.
func (l *ClientLink) registerPurgeable(c *clientConn) {
	l.purgeMu.Lock()
	if l.purgeable == nil {
		l.purgeable = make(map[*clientConn]struct{})
	}
	l.purgeable[c] = struct{}{}
	l.purgeMu.Unlock()
}

// unregisterPurgeable removes c from the purge set.
func (l *ClientLink) unregisterPurgeable(c *clientConn) {
	l.purgeMu.Lock()
	delete(l.purgeable, c)
	l.purgeMu.Unlock()
}

// clientConn is one connected login client. Its handler goroutine owns the
// protocol flow; the purge loop may additionally call kick from outside,
// so sends are serialized by sendMu.
type clientConn struct {
	conn        net.Conn
	remoteIP    net.IP
	crypt       *logincrypt.LoginCrypt
	key         *commoncrypt.LoginKeyPair
	sessionID   int32
	connectedAt time.Time

	account     string
	authed      bool
	ggAuthed    bool
	joinedGame  bool
	accessLevel int
	lastServer  int
	loginKey1   int32
	loginKey2   int32

	sendMu sync.Mutex
}

func (c *clientConn) send(payload []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if wire.FrameHeaderSize+commoncrypt.PaddedSize(len(payload)+8) > wire.MaxFrameLength {
		return fmt.Errorf("login packet exceeds maximum frame length")
	}
	return wire.WriteFrame(c.conn, c.crypt.Encrypt(payload))
}

// kick replies LoginFail AccessFailed and closes the connection; safe to
// call while the handler goroutine is blocked reading.
func (c *clientConn) kick() {
	_ = c.send(serverpackets.EncodeLoginFail(serverpackets.LoginFailAccessFailed))
	c.conn.Close()
}

func (l *ClientLink) handleConnection(ctx context.Context, conn net.Conn) {
	var c *clientConn
	defer func() {
		if c != nil {
			l.unregisterPurgeable(c)
		}
		if c != nil && c.account != "" && !c.joinedGame {
			l.sessions.Delete(c.account)
		}
		conn.Close()
	}()

	ip := remoteIP(conn)
	if l.bans.IsBanned(ip) {
		l.log.Info().Str("ip", ip.String()).Msg("banned login client tried to connect")
		return
	}

	sessionKey, err := l.newSessionKey()
	if err != nil {
		l.log.Error().Err(err).Msg("generate login session key")
		return
	}
	cr, err := logincrypt.NewLoginCrypt(sessionKey)
	if err != nil {
		l.log.Error().Err(err).Msg("build login crypt")
		return
	}

	c = &clientConn{
		conn:        conn,
		remoteIP:    ip,
		crypt:       cr,
		key:         l.newKeyPair(),
		sessionID:   l.newSessionID(),
		connectedAt: time.Now(),
	}

	if err := c.send(serverpackets.EncodeInit(c.sessionID, c.key.ScrambledModulus, sessionKey)); err != nil {
		return
	}

	frames := wire.NewFrameReader(conn)
	for {
		payload, err := frames.ReadFrame()
		if err != nil {
			return
		}
		if err := c.crypt.Decrypt(payload); err != nil {
			l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("login client")
			return
		}
		if len(payload) == 0 {
			return
		}

		switch payload[0] {
		case clientpackets.OpcodeAuthGameGuard:
			if !l.onAuthGameGuard(c, payload) {
				return
			}
		case clientpackets.OpcodeRequestAuthLogin:
			if !c.ggAuthed {
				l.log.Warn().Str("ip", c.remoteIP.String()).Msg("login client sent credentials before the GameGuard exchange")
				continue
			}
			if !l.onRequestAuthLogin(ctx, c, payload) {
				return
			}
		default:
			if !c.authed {
				return
			}
			switch payload[0] {
			case clientpackets.OpcodeRequestServerList:
				if !l.onRequestServerList(c, payload) {
					return
				}
			case clientpackets.OpcodeRequestServerLogin:
				if !l.onRequestServerLogin(ctx, c, payload) {
					return
				}
			default:
				return
			}
		}
	}
}

// onAuthGameGuard validates the client's GameGuard response against the
// Init session id and replies GGAuth. It reports false when the connection
// must close.
func (l *ClientLink) onAuthGameGuard(c *clientConn, payload []byte) bool {
	req, err := clientpackets.DecodeAuthGameGuard(payload)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("login client")
		return true
	}
	if req.SessionID != c.sessionID {
		l.log.Warn().Str("ip", c.remoteIP.String()).Int32("got", req.SessionID).Int32("want", c.sessionID).
			Msg("login client failed the GameGuard session-id check")
		_ = c.send(serverpackets.EncodeLoginFail(serverpackets.LoginFailAccessFailed))
		return false
	}
	c.ggAuthed = true
	_ = c.send(serverpackets.EncodeGGAuth(c.sessionID))
	return true
}

// onRequestAuthLogin authenticates the presented credentials, issues a
// session, and replies LoginOk, or replies LoginFail/AccountKicked and
// reports false when the connection must close.
func (l *ClientLink) onRequestAuthLogin(ctx context.Context, c *clientConn, payload []byte) bool {
	req, err := clientpackets.DecodeRequestAuthLogin(payload, c.key.Private)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("login client")
		_ = c.send(serverpackets.EncodeLoginFail(serverpackets.LoginFailAccessFailed))
		return false
	}

	account, ok := l.authenticate(ctx, c, req)
	if !ok {
		return false
	}

	if account.AccessLevel < 0 {
		_ = c.send(serverpackets.EncodeAccountKicked(serverpackets.AccountKickedPermanentlyBanned))
		return false
	}

	if _, dup := l.sessions.Get(account.Login); dup {
		_ = c.send(serverpackets.EncodeLoginFail(serverpackets.LoginFailAccountInUse))
		return false
	}

	c.account = account.Login
	c.lastServer = account.LastServer
	c.accessLevel = account.AccessLevel
	c.loginKey1, c.loginKey2 = rand.Int32(), rand.Int32()
	l.sessions.Put(c.account, link.SessionKey{LoginKey1: c.loginKey1, LoginKey2: c.loginKey2})
	c.authed = true
	l.registerPurgeable(c)

	if l.skipLicenceCheck {
		return c.send(serverpackets.EncodeServerList(byte(c.lastServer), l.serverEntries(c.accessLevel, c.remoteIP))) == nil
	}
	return c.send(serverpackets.EncodeLoginOk(c.loginKey1, c.loginKey2)) == nil
}

// authenticate resolves req's account, auto-creating it on first login when
// allowed, and verifies its password. It sends the appropriate LoginFail
// itself on any failure, so the caller only needs to check the bool result.
func (l *ClientLink) authenticate(ctx context.Context, c *clientConn, req clientpackets.RequestAuthLogin) (model.Account, bool) {
	account, err := l.accounts.Account(ctx, req.Username)
	switch {
	case errors.Is(err, loginsql.ErrAccountNotFound):
		if !l.autoCreateAccounts {
			l.recordFailedAttempt(c.remoteIP)
			_ = c.send(serverpackets.EncodeLoginFail(serverpackets.LoginFailUserOrPassWrong))
			return model.Account{}, false
		}
		hashed, herr := model.HashPassword(req.Password)
		if herr != nil {
			l.log.Error().Err(herr).Msg("hash password for auto-created account")
			_ = c.send(serverpackets.EncodeLoginFail(serverpackets.LoginFailSystemError))
			return model.Account{}, false
		}
		account, err = l.accounts.CreateAccount(ctx, req.Username, hashed, time.Now())
		if err != nil {
			l.log.Error().Str("account", req.Username).Err(err).Msg("auto-create account")
			_ = c.send(serverpackets.EncodeLoginFail(serverpackets.LoginFailSystemError))
			return model.Account{}, false
		}
		return account, true

	case err != nil:
		l.log.Error().Str("account", req.Username).Err(err).Msg("look up account")
		_ = c.send(serverpackets.EncodeLoginFail(serverpackets.LoginFailSystemError))
		return model.Account{}, false

	default:
		if bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(req.Password)) != nil {
			l.recordFailedAttempt(c.remoteIP)
			_ = c.send(serverpackets.EncodeLoginFail(serverpackets.LoginFailPasswordWrong))
			return model.Account{}, false
		}
		l.clearFailedAttempts(c.remoteIP)
		if err := l.accounts.SetLastActive(ctx, account.Login, time.Now()); err != nil {
			l.log.Error().Str("account", account.Login).Err(err).Msg("set last-active time")
			_ = c.send(serverpackets.EncodeLoginFail(serverpackets.LoginFailAccessFailed))
			return model.Account{}, false
		}
		return account, true
	}
}

func (l *ClientLink) recordFailedAttempt(ip net.IP) {
	key := ip.String()

	l.failedMu.Lock()
	if l.failedAttempts == nil {
		l.failedAttempts = make(map[string]int)
	}
	attempts := l.failedAttempts[key] + 1
	if attempts < l.loginTryBeforeBan {
		l.failedAttempts[key] = attempts
		l.failedMu.Unlock()
		return
	}
	delete(l.failedAttempts, key)
	l.failedMu.Unlock()

	l.bans.Ban(ip, l.loginBlockAfterBan)
	l.log.Info().Str("ip", key).Msg("IP address banned due to too many login attempts")
}

func (l *ClientLink) clearFailedAttempts(ip net.IP) {
	key := ip.String()

	l.failedMu.Lock()
	delete(l.failedAttempts, key)
	l.failedMu.Unlock()
}

// onRequestServerList validates the session keys and replies ServerList.
// It reports false when the connection must close.
func (l *ClientLink) onRequestServerList(c *clientConn, payload []byte) bool {
	req, err := clientpackets.DecodeRequestServerList(payload)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("login client")
		return true
	}
	if req.SessionKey1 != c.loginKey1 || req.SessionKey2 != c.loginKey2 {
		_ = c.send(serverpackets.EncodeLoginFail(serverpackets.LoginFailAccessFailed))
		return false
	}
	entries := l.serverEntries(c.accessLevel, c.remoteIP)
	for _, e := range entries {
		l.log.Info().
			Uint8("id", e.ID).
			Str("ip", net.IP(e.IP[:]).String()).
			Int32("port", e.Port).
			Bool("online", e.Online).
			Msg("serving ServerList entry")
	}
	return c.send(serverpackets.EncodeServerList(byte(c.lastServer), entries)) == nil
}

// serverEntries projects the registry's live server state into the wire
// format, folding in each server's current online-account count. GM-only
// servers are masked as down for accounts at access level 0 or below, and a
// client connecting from a local address is pointed at each game server's
// link connection IP instead of its advertised host.
func (l *ClientLink) serverEntries(accessLevel int, clientIP net.IP) []serverpackets.ServerEntry {
	localClient := isLocalIP(clientIP)
	all := l.servers.All()
	out := make([]serverpackets.ServerEntry, 0, len(all))
	for _, e := range all {
		online := e.Status != link.ServerTypeDown && !(e.Status == link.ServerTypeGMOnly && accessLevel <= 0) && accessLevel >= 0
		host := e.Host
		if localClient && e.ConnIP != nil {
			host = e.ConnIP.String()
		}
		ip := [4]byte{127, 0, 0, 1}
		if parsed := net.ParseIP(host).To4(); parsed != nil {
			copy(ip[:], parsed)
		}
		out = append(out, serverpackets.ServerEntry{
			ID:             byte(e.ID),
			IP:             ip,
			Port:           int32(e.Port),
			AgeLimit:       byte(e.AgeLimit),
			PvP:            e.Pvp,
			CurrentPlayers: uint16(l.servers.OnlineAccountCount(e.ID)),
			MaxPlayers:     uint16(e.MaxPlayers),
			Online:         online,
			TestServer:     e.TestServer,
			ShowClock:      e.ShowClock,
			ShowBrackets:   e.Brackets,
		})
	}
	return out
}

// isLocalIP reports whether addr is missing or points at this machine's
// vicinity (loopback, link-local, unspecified, or site-private), matching
// the reference's local-address test for ServerList host substitution.
func isLocalIP(addr net.IP) bool {
	return addr == nil || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() || addr.IsPrivate()
}

// loginPossible mirrors the account-level gate for entering a game server:
// the server must be linked and authed, its status must not be down, and a
// GM-only or full server admits only superior access levels.
func (l *ClientLink) loginPossible(c *clientConn, serverID int) bool {
	entry, ok := l.servers.Get(serverID)
	if !ok || !entry.Authed || entry.Status == link.ServerTypeDown {
		return false
	}
	if entry.Status == link.ServerTypeGMOnly || l.servers.OnlineAccountCount(serverID) >= int(entry.MaxPlayers) {
		return c.accessLevel > 0
	}
	return c.accessLevel >= 0
}

// onRequestServerLogin validates the session keys where the licence window
// is shown, gates the chosen game server on the account's eligibility, and
// issues the play session. It replies with LoginFail/PlayFail and reports
// false when the connection must close.
func (l *ClientLink) onRequestServerLogin(ctx context.Context, c *clientConn, payload []byte) bool {
	req, err := clientpackets.DecodeRequestServerLogin(payload)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("login client")
		return true
	}
	if !l.skipLicenceCheck && (req.SessionKey1 != c.loginKey1 || req.SessionKey2 != c.loginKey2) {
		_ = c.send(serverpackets.EncodeLoginFail(serverpackets.LoginFailAccessFailed))
		return false
	}

	serverID := int(req.ServerID)
	if !l.loginPossible(c, serverID) {
		_ = c.send(serverpackets.EncodePlayFail(serverpackets.PlayFailTooManyPlayers))
		return false
	}

	playKey1, playKey2 := rand.Int32(), rand.Int32()
	l.sessions.Put(c.account, link.SessionKey{
		LoginKey1: c.loginKey1,
		LoginKey2: c.loginKey2,
		PlayKey1:  playKey1,
		PlayKey2:  playKey2,
	})
	c.joinedGame = true
	if c.lastServer != serverID {
		if err := l.accounts.SetLastServer(ctx, c.account, serverID); err != nil {
			l.log.Error().Str("account", c.account).Err(err).Msg("set last server")
		}
	}
	return c.send(serverpackets.EncodePlayOk(playKey1, playKey2)) == nil
}

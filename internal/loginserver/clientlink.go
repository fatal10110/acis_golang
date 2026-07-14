package loginserver

import (
	"context"
	"errors"
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
)

// accountStore is the account persistence ClientLink needs. *sql.AccountStore
// satisfies it in production; tests substitute an in-memory fake so the
// login flow can be exercised without a database.
type accountStore interface {
	Account(ctx context.Context, login string) (model.Account, error)
	CreateAccount(ctx context.Context, login, hashedPassword string, createdAt time.Time) (model.Account, error)
	SetLastServer(ctx context.Context, login string, serverID int) error
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
	loginTryBeforeBan  int
	loginBlockAfterBan time.Duration
	log                zerolog.Logger

	// failedMu guards failedAttempts.
	failedMu       sync.Mutex
	failedAttempts map[string]int

	// newKeyPair and newSessionKey supply each connection's RSA key pair and
	// dynamic Blowfish key; overridden in tests for a deterministic
	// handshake.
	newKeyPair    func() *commoncrypt.LoginKeyPair
	newSessionKey func() ([]byte, error)
}

// NewClientLink builds a ClientLink from its collaborators. autoCreateAccounts
// mirrors the AutoCreateAccounts config flag: an unrecognized login is
// registered on its first successful RequestAuthLogin rather than rejected.
func NewClientLink(
	accounts *loginsql.AccountStore,
	servers *manager.ServerRegistry,
	sessions *manager.SessionStore,
	bans *manager.IPBanList,
	keys *manager.LoginKeyPool,
	autoCreateAccounts bool,
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
		loginTryBeforeBan:  loginTryBeforeBan,
		loginBlockAfterBan: loginBlockAfterBan,
		log:                log,
		failedAttempts:     make(map[string]int),
		newKeyPair:         keys.Random,
		newSessionKey:      logincrypt.NewSessionKey,
	}
}

// Serve accepts login-client connections on ln until ctx is canceled or
// accepting fails. Each connection is handled on its own goroutine. The
// caller owns ln: Serve closes it on ctx cancellation but does not create
// it, so tests can bind an ephemeral port.
func (l *ClientLink) Serve(ctx context.Context, ln net.Listener) error {
	return netutil.AcceptLoop(ctx, ln, func(conn net.Conn) {
		l.handleConnection(ctx, conn)
	}, l.log)
}

// clientConn is one connected login client. It is owned entirely by the
// goroutine running handleConnection: nothing else writes to conn or
// advances crypt.
type clientConn struct {
	conn     net.Conn
	remoteIP net.IP
	crypt    *logincrypt.LoginCrypt
	key      *commoncrypt.LoginKeyPair

	account    string
	authed     bool
	joinedGame bool
	lastServer int
	loginKey1  int32
	loginKey2  int32
}

func (c *clientConn) send(payload []byte) error {
	return wire.WriteFrame(c.conn, c.crypt.Encrypt(payload))
}

func (l *ClientLink) handleConnection(ctx context.Context, conn net.Conn) {
	var c *clientConn
	defer func() {
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
		conn:     conn,
		remoteIP: ip,
		crypt:    cr,
		key:      l.newKeyPair(),
	}

	if err := c.send(serverpackets.EncodeInit(rand.Int32(), c.key.ScrambledModulus, sessionKey)); err != nil {
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
			l.onAuthGameGuard(c, payload)
		case clientpackets.OpcodeRequestAuthLogin:
			if !l.onRequestAuthLogin(ctx, c, payload) {
				return
			}
		default:
			if !c.authed {
				return
			}
			switch payload[0] {
			case clientpackets.OpcodeRequestServerList:
				l.onRequestServerList(c, payload)
			case clientpackets.OpcodeRequestServerLogin:
				l.onRequestServerLogin(ctx, c, payload)
			default:
				return
			}
		}
	}
}

func (l *ClientLink) onAuthGameGuard(c *clientConn, payload []byte) {
	if _, err := clientpackets.DecodeAuthGameGuard(payload); err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("login client")
		return
	}
	_ = c.send(serverpackets.EncodeGGAuth(serverpackets.GGAuthSkipRequest))
}

// onRequestAuthLogin authenticates the presented credentials, issues a
// session, and replies LoginOk, or replies LoginFail/AccountKicked and
// reports false when the connection must close.
func (l *ClientLink) onRequestAuthLogin(ctx context.Context, c *clientConn, payload []byte) bool {
	req, err := clientpackets.DecodeRequestAuthLogin(payload, c.key.Private)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("login client")
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
	c.loginKey1, c.loginKey2 = rand.Int32(), rand.Int32()
	l.sessions.Put(c.account, link.SessionKey{LoginKey1: c.loginKey1, LoginKey2: c.loginKey2})
	c.authed = true

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
			_ = c.send(serverpackets.EncodeLoginFail(serverpackets.LoginFailUserOrPassWrong))
			return model.Account{}, false
		}
		l.clearFailedAttempts(c.remoteIP)
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

func (l *ClientLink) onRequestServerList(c *clientConn, payload []byte) {
	req, err := clientpackets.DecodeRequestServerList(payload)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("login client")
		return
	}
	if req.SessionKey1 != c.loginKey1 || req.SessionKey2 != c.loginKey2 {
		return
	}
	entries := l.serverEntries()
	for _, e := range entries {
		l.log.Info().
			Uint8("id", e.ID).
			Str("ip", net.IP(e.IP[:]).String()).
			Int32("port", e.Port).
			Bool("online", e.Online).
			Msg("serving ServerList entry")
	}
	_ = c.send(serverpackets.EncodeServerList(byte(c.lastServer), entries))
}

// serverEntries projects the registry's live server state into the wire
// format, folding in each server's current online-account count.
func (l *ClientLink) serverEntries() []serverpackets.ServerEntry {
	all := l.servers.All()
	out := make([]serverpackets.ServerEntry, 0, len(all))
	for _, e := range all {
		ip := [4]byte{127, 0, 0, 1}
		if parsed := net.ParseIP(e.Host).To4(); parsed != nil {
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
			Online:         e.Status != link.ServerTypeDown,
			TestServer:     e.TestServer,
			ShowClock:      e.ShowClock,
			ShowBrackets:   e.Brackets,
		})
	}
	return out
}

func (l *ClientLink) onRequestServerLogin(ctx context.Context, c *clientConn, payload []byte) {
	req, err := clientpackets.DecodeRequestServerLogin(payload)
	if err != nil {
		l.log.Warn().Str("ip", c.remoteIP.String()).Err(err).Msg("login client")
		return
	}
	if req.SessionKey1 != c.loginKey1 || req.SessionKey2 != c.loginKey2 {
		_ = c.send(serverpackets.EncodePlayFail(serverpackets.PlayFailSystemError))
		return
	}

	entry, ok := l.servers.Get(int(req.ServerID))
	if !ok || !entry.Authed {
		_ = c.send(serverpackets.EncodePlayFail(serverpackets.PlayFailSystemError))
		return
	}

	playKey1, playKey2 := rand.Int32(), rand.Int32()
	l.sessions.Put(c.account, link.SessionKey{
		LoginKey1: c.loginKey1,
		LoginKey2: c.loginKey2,
		PlayKey1:  playKey1,
		PlayKey2:  playKey2,
	})
	c.joinedGame = true
	if err := l.accounts.SetLastServer(ctx, c.account, int(req.ServerID)); err != nil {
		l.log.Error().Str("account", c.account).Err(err).Msg("set last server")
	}
	_ = c.send(serverpackets.EncodePlayOk(playKey1, playKey2))
}

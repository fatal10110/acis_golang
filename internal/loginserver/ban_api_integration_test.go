package loginserver

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	commoncrypt "github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/debughttp"
	"github.com/fatal10110/acis_golang/internal/commons/netutil"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/serverpackets"
)

type banAPIRow struct {
	IP        string `json:"ip"`
	Store     string `json:"store"`
	Permanent bool   `json:"permanent"`
	Expires   string `json:"expires"`
}

// banAPIClient drives the ban API as an operator's tool would.
type banAPIClient struct {
	t   *testing.T
	url string
}

func (c banAPIClient) list() []banAPIRow {
	c.t.Helper()
	resp, err := http.Get(c.url + debughttp.BansPath)
	if err != nil {
		c.t.Fatalf("GET %s: %v", debughttp.BansPath, err)
	}
	defer resp.Body.Close()
	var out struct {
		Server string      `json:"server"`
		Bans   []banAPIRow `json:"bans"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK || out.Server != "login" {
		c.t.Fatalf("GET %s: status %d server %q err %v", debughttp.BansPath, resp.StatusCode, out.Server, err)
	}
	return out.Bans
}

// post sends body to path and returns the named boolean of the answer.
func (c banAPIClient) post(path, body, field string) bool {
	c.t.Helper()
	resp, err := http.Post(c.url+path, "application/json", strings.NewReader(body))
	if err != nil {
		c.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	var out map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK {
		c.t.Fatalf("POST %s %s: status %d err %v", path, body, resp.StatusCode, err)
	}
	return out[field]
}

// dialLoginClientFrom is dialLoginClient from the local address local.
func dialLoginClientFrom(t *testing.T, addr, local string) *fakeLoginClient {
	t.Helper()
	d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(local)}}
	conn, err := d.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s from %s: %v", addr, local, err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := wire.ReadFrame(conn); err != nil {
		t.Fatalf("read Init frame from %s: %v", local, err)
	}
	cipher, err := commoncrypt.NewBlowfishCipher(testSessionKey)
	if err != nil {
		t.Fatalf("NewBlowfishCipher: %v", err)
	}
	return &fakeLoginClient{t: t, conn: conn, cipher: cipher}
}

// refusedFrom connects from local and requires a close before Init.
func refusedFrom(t *testing.T, addr, local string) {
	t.Helper()
	d := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(local)}}
	conn, err := d.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s from %s: %v", addr, local, err)
	}
	t.Cleanup(func() { conn.Close() })
	(&fakeLoginClient{t: t, conn: conn}).expectClosed()
}

// findBan returns the row for ip in store, failing when rows hold none.
func findBan(t *testing.T, rows []banAPIRow, ip, store string) banAPIRow {
	t.Helper()
	for _, r := range rows {
		if r.IP == ip && r.Store == store {
			return r
		}
	}
	t.Fatalf("no %s ban for %s in %+v", store, ip, rows)
	return banAPIRow{}
}

// Each login-port ban type is listed by the ban API, lifted through it, and
// the address's next connection gets Init: the failed-login ban, the accept
// filter's burst ban, and a permanent ban added through the API.
func TestBanAPIListsAndLiftsLoginBans(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Unix(1_000_000, 0).UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()) }
	advance := func(d time.Duration) { clock.Add(int64(d)) }

	filter := netutil.NewAcceptFilter(netutil.DefaultAcceptFilterConfig(), zerolog.Nop())
	accounts := newFakeAccountStore(model.NewAccount("player1", mustHashPassword(t, "s3cret"), 0, 1))
	addr, l, _, _, bans := newTestClientLinkOn(t, func(ln net.Listener) net.Listener {
		return netutil.FilterAccepts(ln, filter, now)
	}, accounts, false)
	ts := httptest.NewServer(debughttp.BanHandler("login", []debughttp.BanStore{
		{Name: "login-ban", Bans: bans},
		{Name: "login-accept-filter", Bans: filter},
	}, zerolog.Nop()))
	t.Cleanup(ts.Close)
	api := banAPIClient{t: t, url: ts.URL}

	if rows := api.list(); len(rows) != 0 {
		t.Fatalf("fresh server lists bans %+v", rows)
	}

	// Failed logins, spaced past the burst window, ban 127.0.0.1.
	for range DefaultLoginTryBeforeBan {
		advance(2 * time.Second)
		c := dialLoginClientFrom(t, addr, "127.0.0.1")
		c.gameGuard()
		c.send(encodeRequestAuthLogin(&l.loginKeyPair().Private.PublicKey, "player1", "wrong"))
		c.expectLoginFail(serverpackets.LoginFailPasswordWrong)
	}
	advance(2 * time.Second)
	refusedFrom(t, addr, "127.0.0.1")
	row := findBan(t, api.list(), "127.0.0.1", "login-ban")
	if expires, err := time.Parse(time.RFC3339, row.Expires); row.Permanent || err != nil ||
		expires.Before(time.Now().Add(DefaultLoginBlockAfterBan-time.Minute)) {
		t.Fatalf("failed-login ban row %+v, want a temporary ban about %v out", row, DefaultLoginBlockAfterBan)
	}
	if !api.post(debughttp.BansRemovePath, `{"ip":"127.0.0.1","store":"login-ban"}`, "removed") {
		t.Fatal("remove of the failed-login ban reported none")
	}
	advance(2 * time.Second)
	dialLoginClientFrom(t, addr, "127.0.0.1")

	// A burst from 127.0.0.2 bans it at accept.
	for range 4 {
		advance(100 * time.Millisecond)
		dialLoginClientFrom(t, addr, "127.0.0.2")
	}
	advance(100 * time.Millisecond)
	refusedFrom(t, addr, "127.0.0.2")
	row = findBan(t, api.list(), "127.0.0.2", "login-accept-filter")
	if want := now().Add(300 * time.Second).UTC().Format(time.RFC3339); row.Permanent || row.Expires != want {
		t.Fatalf("accept-filter ban row %+v, want expiry %s", row, want)
	}
	if !api.post(debughttp.BansRemovePath, `{"ip":"127.0.0.2","store":"login-accept-filter"}`, "removed") {
		t.Fatal("remove of the accept-filter ban reported none")
	}
	advance(100 * time.Millisecond)
	dialLoginClientFrom(t, addr, "127.0.0.2")

	// An operator's permanent ban on 127.0.0.3.
	if !api.post(debughttp.BansAddPath, `{"ip":"127.0.0.3","store":"login-ban","permanent":true}`, "added") {
		t.Fatal("add of a permanent ban reported none")
	}
	refusedFrom(t, addr, "127.0.0.3")
	if row := findBan(t, api.list(), "127.0.0.3", "login-ban"); !row.Permanent || row.Expires != "" {
		t.Fatalf("permanent ban row %+v", row)
	}
	if !api.post(debughttp.BansRemovePath, `{"ip":"127.0.0.3","store":"login-ban"}`, "removed") {
		t.Fatal("remove of the permanent ban reported none")
	}
	advance(2 * time.Second)
	dialLoginClientFrom(t, addr, "127.0.0.3")

	if rows := api.list(); len(rows) != 0 {
		t.Fatalf("bans left after every remove: %+v", rows)
	}
}

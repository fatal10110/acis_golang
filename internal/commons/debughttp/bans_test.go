package debughttp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/netutil"
)

var banEpoch = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

// testBanAPI serves the ban API over a ban list timed at banEpoch and an
// accept filter that bans 198.51.100.9 at banEpoch.
func testBanAPI(t *testing.T) (*httptest.Server, *netutil.BanList) {
	t.Helper()
	bans := netutil.NewBanList(func() time.Time { return banEpoch })
	filter := netutil.NewAcceptFilter(netutil.DefaultAcceptFilterConfig(), zerolog.Nop())
	for range 5 {
		filter.Allow(netip.MustParseAddr("198.51.100.9"), banEpoch)
	}
	h := BanHandler("game", []BanStore{
		{Name: "game-ban", Bans: bans},
		{Name: "game-accept-filter", Bans: filter},
	}, zerolog.Nop())
	ts := httptest.NewServer(routes(h))
	t.Cleanup(ts.Close)
	return ts, bans
}

func do(t *testing.T, method, url, contentType, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, strings.TrimSpace(string(b))
}

func TestBanAPIListAddRemove(t *testing.T) {
	ts, _ := testBanAPI(t)
	const jsonType = "application/json; charset=utf-8"

	for _, add := range []string{
		`{"ip":"203.0.113.7","store":"game-ban","permanent":true}`,
		`{"ip":"::ffff:203.0.113.8","store":"game-ban","seconds":600}`,
	} {
		if code, body := do(t, http.MethodPost, ts.URL+BansAddPath, jsonType, add); code != http.StatusOK || body != `{"added":true}` {
			t.Fatalf("add %s = %d %s", add, code, body)
		}
	}
	if code, body := do(t, http.MethodPost, ts.URL+BansAddPath, "application/json", `{"ip":"203.0.113.7","store":"game-ban","seconds":5}`); code != http.StatusOK || body != `{"added":false}` {
		t.Fatalf("add over a ban = %d %s, want added false", code, body)
	}

	want := `{"server":"game","stores":[{"name":"game-ban","add":true},{"name":"game-accept-filter","add":false}],"bans":[` +
		`{"ip":"203.0.113.7","store":"game-ban","permanent":true},` +
		`{"ip":"203.0.113.8","store":"game-ban","permanent":false,"expires":"2026-11-06T12:10:00Z"},` +
		`{"ip":"198.51.100.9","store":"game-accept-filter","permanent":false,"expires":"2026-11-06T12:05:00Z"}]}`
	if code, body := do(t, http.MethodGet, ts.URL+BansPath, "", ""); code != http.StatusOK || body != want {
		t.Fatalf("list = %d\n%s\nwant\n%s", code, body, want)
	}

	for _, rm := range []struct{ body, want string }{
		{`{"ip":"203.0.113.7","store":"game-ban"}`, `{"removed":true}`},
		{`{"ip":"203.0.113.7","store":"game-ban"}`, `{"removed":false}`},
		{`{"ip":"198.51.100.9","store":"game-accept-filter"}`, `{"removed":true}`},
	} {
		if code, body := do(t, http.MethodPost, ts.URL+BansRemovePath, "application/json", rm.body); code != http.StatusOK || body != rm.want {
			t.Fatalf("remove %s = %d %s, want %s", rm.body, code, body, rm.want)
		}
	}
	want = `{"server":"game","stores":[{"name":"game-ban","add":true},{"name":"game-accept-filter","add":false}],"bans":[` +
		`{"ip":"203.0.113.8","store":"game-ban","permanent":false,"expires":"2026-11-06T12:10:00Z"}]}`
	if _, body := do(t, http.MethodGet, ts.URL+BansPath, "", ""); body != want {
		t.Fatalf("list after removes = %s, want %s", body, want)
	}
}

func TestBanAPIRejectsBadRequests(t *testing.T) {
	ts, bans := testBanAPI(t)
	for _, tc := range []struct {
		name, method, path, ctype, body string
		code                            int
		errText                         string
	}{
		{"list by POST", http.MethodPost, BansPath, "application/json", `{}`, http.StatusMethodNotAllowed, "method not allowed"},
		{"add by GET", http.MethodGet, BansAddPath, "", "", http.StatusMethodNotAllowed, "method not allowed"},
		{"remove as form", http.MethodPost, BansRemovePath, "application/x-www-form-urlencoded", "ip=1.2.3.4", http.StatusUnsupportedMediaType, "content type"},
		{"add without type", http.MethodPost, BansAddPath, "", `{"ip":"1.2.3.4","store":"game-ban","permanent":true}`, http.StatusUnsupportedMediaType, "content type"},
		{"bad json", http.MethodPost, BansAddPath, "application/json", `{"ip":`, http.StatusBadRequest, "bad json"},
		{"unknown field", http.MethodPost, BansAddPath, "application/json", `{"ip":"1.2.3.4","store":"game-ban","duration":60}`, http.StatusBadRequest, "bad json"},
		{"trailing data", http.MethodPost, BansAddPath, "application/json", `{"ip":"1.2.3.4","store":"game-ban","permanent":true}{}`, http.StatusBadRequest, "trailing"},
		{"too large", http.MethodPost, BansAddPath, "application/json", `{"ip":"` + strings.Repeat("1", maxBanBody) + `"}`, http.StatusRequestEntityTooLarge, "too large"},
		{"hostname", http.MethodPost, BansAddPath, "application/json", `{"ip":"example.com","store":"game-ban","permanent":true}`, http.StatusBadRequest, "literal IP"},
		{"zoned", http.MethodPost, BansAddPath, "application/json", `{"ip":"fe80::1%eth0","store":"game-ban","permanent":true}`, http.StatusBadRequest, "literal IP"},
		{"unknown store", http.MethodPost, BansRemovePath, "application/json", `{"ip":"1.2.3.4","store":"login-ban"}`, http.StatusBadRequest, "unknown store"},
		{"filter add", http.MethodPost, BansAddPath, "application/json", `{"ip":"1.2.3.4","store":"game-accept-filter","permanent":true}`, http.StatusBadRequest, "does not take bans"},
		{"no duration", http.MethodPost, BansAddPath, "application/json", `{"ip":"1.2.3.4","store":"game-ban"}`, http.StatusBadRequest, "seconds must be"},
		{"both", http.MethodPost, BansAddPath, "application/json", `{"ip":"1.2.3.4","store":"game-ban","permanent":true,"seconds":5}`, http.StatusBadRequest, "not both"},
		{"negative", http.MethodPost, BansAddPath, "application/json", `{"ip":"1.2.3.4","store":"game-ban","seconds":-5}`, http.StatusBadRequest, "seconds must be"},
		{"too long", http.MethodPost, BansAddPath, "application/json", `{"ip":"1.2.3.4","store":"game-ban","seconds":315360001}`, http.StatusBadRequest, "seconds must be"},
		{"unknown path", http.MethodGet, BansPath + "/x", "", "", http.StatusNotFound, "not found"},
	} {
		code, body := do(t, tc.method, ts.URL+tc.path, tc.ctype, tc.body)
		if code != tc.code || !strings.HasPrefix(body, `{"error":"`) || !strings.Contains(body, tc.errText) {
			t.Errorf("%s: %d %s, want %d with error containing %q", tc.name, code, body, tc.code, tc.errText)
		}
	}
	if got := bans.List(); len(got) != 0 {
		t.Fatalf("rejected requests banned %v", got)
	}
}

// A request from off the host is refused even when the listener is bound
// to a public address.
func TestBanAPIRefusesNonLoopbackPeers(t *testing.T) {
	h := BanHandler("login", nil, zerolog.Nop())
	for _, remote := range []string{"203.0.113.5:40000", "[2001:db8::1]:40000", "bogus"} {
		req := httptest.NewRequest(http.MethodGet, BansPath, nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("peer %s: status %d, want 403", remote, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, BansPath, nil)
	req.RemoteAddr = "[::1]:40000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"server":"login","stores":[],"bans":[]}` {
		t.Fatalf("loopback peer: %d %s", rec.Code, rec.Body.String())
	}
}

// The debug listener keeps pprof and expvar beside the ban API.
func TestRoutesKeepDebugEndpoints(t *testing.T) {
	ts, _ := testBanAPI(t)
	if code, _ := do(t, http.MethodGet, ts.URL+"/debug/vars", "", ""); code != http.StatusOK {
		t.Fatalf("/debug/vars status %d beside the ban API", code)
	}
	if code, _ := do(t, http.MethodGet, ts.URL+"/debug/pprof/", "", ""); code != http.StatusOK {
		t.Fatalf("/debug/pprof/ status %d beside the ban API", code)
	}
}

package lifecycle

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/debughttp"
	"github.com/fatal10110/acis_golang/internal/commons/netutil"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

type banRow struct {
	IP        string `json:"ip"`
	Store     string `json:"store"`
	Permanent bool   `json:"permanent"`
	Expires   string `json:"expires"`
}

// banAPI drives the game server's ban API as an operator's tool would.
type banAPI struct {
	t   *testing.T
	url string
}

func (a banAPI) list() []banRow {
	a.t.Helper()
	resp, err := http.Get(a.url + debughttp.BansPath)
	if err != nil {
		a.t.Fatalf("GET %s: %v", debughttp.BansPath, err)
	}
	defer resp.Body.Close()
	var out struct {
		Server string   `json:"server"`
		Bans   []banRow `json:"bans"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK || out.Server != "game" {
		a.t.Fatalf("GET %s: status %d server %q err %v", debughttp.BansPath, resp.StatusCode, out.Server, err)
	}
	return out.Bans
}

// post sends body to path and returns the named boolean of the answer.
func (a banAPI) post(path, body, field string) bool {
	a.t.Helper()
	resp, err := http.Post(a.url+path, "application/json", strings.NewReader(body))
	if err != nil {
		a.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	var out map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK {
		a.t.Fatalf("POST %s %s: status %d err %v", path, body, resp.StatusCode, err)
	}
	return out[field]
}

// requireBans requires the list to be exactly want, in the API's order.
func (a banAPI) requireBans(want ...banRow) {
	a.t.Helper()
	got := a.list()
	if len(got) != len(want) {
		a.t.Fatalf("bans = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			a.t.Fatalf("bans = %+v, want %+v", got, want)
		}
	}
}

// bootWithBanAPI boots a server whose game-client listener runs a ban list
// and an accept filter on one fake clock, and serves the ban API over both.
func bootWithBanAPI(t *testing.T) (*gameservertest.Server, banAPI, *filterClock) {
	t.Helper()
	clock := &filterClock{}
	clock.set(time.Unix(1_000_000, 0))
	filter := netutil.NewAcceptFilter(netutil.DefaultAcceptFilterConfig(), zerolog.Nop())
	bans := netutil.NewBanList(clock.now)
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1),
		gameservertest.WithAcceptFilter(filter, clock.now), gameservertest.WithBanList(bans))
	ts := httptest.NewServer(debughttp.BanHandler("game", []debughttp.BanStore{
		{Name: "game-ban", Bans: bans},
		{Name: "game-accept-filter", Bans: filter},
	}, zerolog.Nop()))
	t.Cleanup(ts.Close)
	return srv, banAPI{t: t, url: ts.URL}, clock
}

func rfc3339(at time.Time) string { return at.UTC().Format(time.RFC3339) }

// The accept filter's burst ban is listed with its sliding expiry, lifted
// through the API, and the address's next connection is answered.
func TestBanAPILiftsGameAcceptFilterBan(t *testing.T) {
	t.Parallel()
	srv, api, clock := bootWithBanAPI(t)

	for range 4 {
		clock.advance(100 * time.Millisecond)
		assertHandshake(t, srv, "127.0.0.2")
	}
	clock.advance(100 * time.Millisecond)
	assertRefused(t, srv, "127.0.0.2")
	api.requireBans(banRow{IP: "127.0.0.2", Store: "game-accept-filter", Expires: rfc3339(clock.now().Add(300 * time.Second))})

	if !api.post(debughttp.BansRemovePath, `{"ip":"127.0.0.2","store":"game-accept-filter"}`, "removed") {
		t.Fatal("remove of the accept-filter ban reported none")
	}
	api.requireBans()
	clock.advance(100 * time.Millisecond)
	assertHandshake(t, srv, "127.0.0.2")
}

// A ban added through the API refuses the address at accept, ahead of the
// accept filter, until it is removed or expires.
func TestBanAPIGameBanRefusesUntilLifted(t *testing.T) {
	t.Parallel()
	srv, api, clock := bootWithBanAPI(t)

	if !api.post(debughttp.BansAddPath, `{"ip":"127.0.0.3","store":"game-ban","permanent":true}`, "added") {
		t.Fatal("add of a permanent ban reported none")
	}
	if !api.post(debughttp.BansAddPath, `{"ip":"127.0.0.4","store":"game-ban","seconds":60}`, "added") {
		t.Fatal("add of a temporary ban reported none")
	}
	if api.post(debughttp.BansAddPath, `{"ip":"127.0.0.4","store":"game-ban","permanent":true}`, "added") {
		t.Fatal("add over a ban in force replaced it")
	}
	api.requireBans(
		banRow{IP: "127.0.0.3", Store: "game-ban", Permanent: true},
		banRow{IP: "127.0.0.4", Store: "game-ban", Expires: rfc3339(clock.now().Add(time.Minute))},
	)

	// Refused attempts from a banned address never reach the accept
	// filter, so a burst of them leaves no filter ban behind.
	for range 6 {
		clock.advance(10 * time.Millisecond)
		assertRefused(t, srv, "127.0.0.3")
	}
	assertRefused(t, srv, "127.0.0.4")

	if !api.post(debughttp.BansRemovePath, `{"ip":"127.0.0.3","store":"game-ban"}`, "removed") {
		t.Fatal("remove of the permanent ban reported none")
	}
	assertHandshake(t, srv, "127.0.0.3")

	// The temporary ban holds to its expiry and lifts after it.
	clock.advance(time.Minute - 60*time.Millisecond)
	assertRefused(t, srv, "127.0.0.4")
	clock.advance(time.Millisecond)
	assertHandshake(t, srv, "127.0.0.4")
	api.requireBans()
}

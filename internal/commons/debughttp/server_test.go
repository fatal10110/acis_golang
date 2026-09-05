package debughttp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMuxServesExpvarAndPprof(t *testing.T) {
	SetPlayersOnlineFunc(func() int { return 3 })
	t.Cleanup(func() { SetPlayersOnlineFunc(func() int { return 0 }) })

	ts := httptest.NewServer(http.DefaultServeMux)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/debug/vars")
	if err != nil {
		t.Fatalf("GET /debug/vars: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /debug/vars status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /debug/vars: %v", err)
	}
	var vars map[string]json.RawMessage
	if err := json.Unmarshal(body, &vars); err != nil {
		t.Fatalf("decode /debug/vars: %v\n%s", err, body)
	}
	if string(vars["players-online"]) != "3" {
		t.Errorf("players-online = %s, want 3", vars["players-online"])
	}

	resp, err = http.Get(ts.URL + "/debug/pprof/goroutine?debug=1")
	if err != nil {
		t.Fatalf("GET /debug/pprof/goroutine: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /debug/pprof/goroutine status = %d", resp.StatusCode)
	}
	profile, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read goroutine profile: %v", err)
	}
	if !strings.Contains(string(profile), "goroutine") {
		t.Errorf("goroutine profile missing expected text:\n%s", profile)
	}
}

func TestListenEmptyAddrIsNoop(t *testing.T) {
	srv, err := Listen("")
	if err != nil {
		t.Fatalf("Listen empty: %v", err)
	}
	if srv != nil {
		t.Fatal("Listen empty: got server, want nil")
	}
}

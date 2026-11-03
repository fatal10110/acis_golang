package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/botclient"
)

func TestRunRejectsBadFlags(t *testing.T) {
	for _, args := range [][]string{
		{"-server", "300"},
		{"-behaviors", "move,dance"},
		{"-prefix", "Bot"},
		{"-no-such-flag"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), args, &stdout, &stderr); code != 2 {
			t.Errorf("run(%q) = %d, want 2 (stderr %q)", args, code, stderr.String())
		}
	}
}

func TestWriteJSONKeys(t *testing.T) {
	r := botclient.Report{
		Bots:        2,
		Started:     2,
		Connect:     botclient.Latency{Count: 2, P50ms: 1.5, P99ms: 2, Maxms: 2},
		Actions:     map[string]int64{"move": 3},
		Errors:      map[string]int64{"login": 1},
		Entered:     1,
		PacketsSent: 10,
	}
	var out bytes.Buffer
	if err := writeJSON("-", &out, &r); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"bots", "started", "entered_world", "elapsed_seconds",
		"connect_latency", "login_latency", "enter_world_latency",
		"packets_sent", "packets_received", "packets_sent_per_second", "packets_received_per_second",
		"actions", "disconnects", "errors", "aborted",
	} {
		if _, ok := doc[key]; !ok {
			t.Errorf("JSON lacks %q: %s", key, strings.TrimSpace(out.String()))
		}
	}
	connect, _ := doc["connect_latency"].(map[string]any)
	if connect["p50_ms"] != 1.5 || connect["p99_ms"] != 2.0 || connect["count"] != 2.0 || connect["max_ms"] != 2.0 {
		t.Errorf("connect_latency = %v", connect)
	}
	if doc["errors"].(map[string]any)["login"] != 1.0 {
		t.Errorf("errors = %v", doc["errors"])
	}
}

package admin

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// TestGrant pins the level a granted one resolves to and what it logs: an
// undefined level reads as the user level with a warning, every negative
// level reads the -1 entry, and the master level is logged.
func TestGrant(t *testing.T) {
	d, err := NewData([]AccessLevel{{Level: -1, Name: "Banned"}, {Level: 0, Name: "User"}, {Level: 8, Name: "Master"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		level     int
		wantLevel int
		wantLog   string
	}{
		{0, 0, ""},
		{8, 8, "master access level"},
		{99, 0, "undefined level granted"},
		{-5, -1, ""},
	} {
		var buf bytes.Buffer
		got := d.Grant(tt.level, "Newbie", zerolog.New(&buf))
		if got.Level != tt.wantLevel {
			t.Errorf("Grant(%d).Level = %d, want %d", tt.level, got.Level, tt.wantLevel)
		}
		if logs := buf.String(); (tt.wantLog == "") != (logs == "") || !strings.Contains(logs, tt.wantLog) {
			t.Errorf("Grant(%d) log = %q, want %q", tt.level, logs, tt.wantLog)
		}
	}
}

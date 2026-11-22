package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
)

// A dump's walk record is replayed through the movement update's stepping.
// An empty geodata directory loads no region, whose floor never matches the
// height a step lands on, so the first update is refused and the walker
// stays where it stood; the engine-level walks are covered in probe.
func TestRun_CompareWalkRecord(t *testing.T) {
	x, y := engine.WorldXMin, engine.WorldYMin
	id := fmt.Sprintf("walk:%d,%d,0->%d,%d,0;speed=200;start=100;steps=100,100,100", x, y, x+100, y)
	start := fmt.Sprintf("%d,%d,0", x, y)

	for name, tc := range map[string]struct {
		line string
		code int
	}{
		"agrees":    {fmt.Sprintf("%s\tblocked=true\tpoints=%s\treached=false\tstop=%s\n", id, start, start), exitOK},
		"disagrees": {fmt.Sprintf("%s\tblocked=false\tpoints=%s\treached=false\tstop=%s\n", id, start, start), exitDiffFound},
	} {
		t.Run(name, func(t *testing.T) {
			expectedPath := writeFile(t, tc.line)
			var stdout, stderr bytes.Buffer
			code := run([]string{"-geodata", t.TempDir(), "-allow-empty-geodata", "-expected-dump", expectedPath}, &stdout, &stderr)
			if code != tc.code {
				t.Fatalf("exit = %d, want %d; stdout=%s stderr=%s", code, tc.code, stdout.String(), stderr.String())
			}
			if tc.code == exitDiffFound && !strings.Contains(stdout.String(), `field blocked: expected="false" got="true"`) {
				t.Errorf("report missing field mismatch:\n%s", stdout.String())
			}
		})
	}
}

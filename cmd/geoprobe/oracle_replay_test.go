package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestOracleDumpReplaysAgainstShippedGeodata replays the committed
// Java-captured dump (testdata/oracle/java_oracle.dump: canmove, los, height,
// validlocation and path queries) against the shipped geodata and fails on any
// disagreement with the recorded Java answers. Local only: the geodata is not
// in the repository, so without it the test skips.
func TestOracleDumpReplaysAgainstShippedGeodata(t *testing.T) {
	geodata := datapack.Geodata(t)
	dump := filepath.Join("testdata", "oracle", "java_oracle.dump")
	raw, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"canmove:", "los:", "height:", "validlocation:", "path:"} {
		if n := strings.Count(string(raw), "\n"+kind); n < 100 {
			t.Fatalf("oracle dump holds %d %s queries, want at least 100", n, kind)
		}
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-geodata", geodata, "-geotype", "L2OFF", "-expected-dump", dump}, &stdout, &stderr)
	if code != exitOK {
		out := stdout.String()
		if len(out) > 4000 {
			out = out[:4000] + "\n..."
		}
		t.Fatalf("replaying the Java oracle dump exit = %d, want %d:\n%s", code, exitOK, out)
	}
	if !strings.Contains(stdout.String(), "agreement=100.00%") {
		t.Fatalf("replay report lacks full agreement:\n%.2000s", stdout.String())
	}
}

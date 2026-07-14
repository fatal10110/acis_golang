package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
)

func TestRun_MissingGeodata(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-queries", "4"}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("exit = %d, want %d; stderr=%s", code, exitError, stderr.String())
	}
}

func TestRun_EmptyGeodataDirErrorsClearly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-geodata", t.TempDir(), "-queries", "4"}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("exit = %d, want %d; stdout=%s stderr=%s", code, exitError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "no L2OFF geodata files found") {
		t.Fatalf("stderr = %q, want clear missing-assets message", stderr.String())
	}
}

func TestRun_UnknownGeoType(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-geodata", t.TempDir(), "-geotype", "bogus"}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("exit = %d, want %d; stderr=%s", code, exitError, stderr.String())
	}
}

func TestRun_GeneratesRandomSampleToStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-geodata", t.TempDir(), "-allow-empty-geodata", "-queries", "8", "-seed", "1"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 8 {
		t.Fatalf("got %d dump lines, want 8:\n%s", len(lines), stdout.String())
	}
}

func TestRun_SameSeedIsReproducible(t *testing.T) {
	dir := t.TempDir()

	var first, second, stderr bytes.Buffer
	if code := run([]string{"-geodata", dir, "-allow-empty-geodata", "-queries", "20", "-seed", "5"}, &first, &stderr); code != exitOK {
		t.Fatalf("first run exit = %d; stderr=%s", code, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"-geodata", dir, "-allow-empty-geodata", "-queries", "20", "-seed", "5"}, &second, &stderr); code != exitOK {
		t.Fatalf("second run exit = %d; stderr=%s", code, stderr.String())
	}
	if first.String() != second.String() {
		t.Fatal("two runs with the same seed produced different dumps")
	}
}

func TestRun_CompareAgainstOwnDumpAgrees(t *testing.T) {
	dir := t.TempDir()

	var dump, stderr bytes.Buffer
	if code := run([]string{"-geodata", dir, "-allow-empty-geodata", "-queries", "12", "-seed", "9"}, &dump, &stderr); code != exitOK {
		t.Fatalf("generate exit = %d; stderr=%s", code, stderr.String())
	}
	expectedPath := writeFile(t, dump.String())

	var stdout bytes.Buffer
	stderr.Reset()
	code := run([]string{"-geodata", dir, "-allow-empty-geodata", "-expected-dump", expectedPath}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d; stderr=%s stdout=%s", code, exitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "queries=12 agreement=100.00%") {
		t.Errorf("report missing full agreement:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "no differences") {
		t.Errorf("report missing no-differences line:\n%s", stdout.String())
	}
}

func TestRun_CompareAgainstDivergentOracleReportsDisagreement(t *testing.T) {
	dir := t.TempDir()

	x, y := engine.WorldXMin, engine.WorldYMin
	id := fmt.Sprintf("height:%d,%d,0", x, y)
	// Every point in an all-null engine echoes back its queried Z (0), so a
	// dump claiming a different height is a deliberate, known-wrong oracle
	// answer for this one query.
	expectedPath := writeFile(t, id+"\theight=999\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-geodata", dir, "-allow-empty-geodata", "-expected-dump", expectedPath}, &stdout, &stderr)
	if code != exitDiffFound {
		t.Fatalf("exit = %d, want %d; stdout=%s stderr=%s", code, exitDiffFound, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "agreement=0.00%") {
		t.Errorf("report missing 0%% agreement:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "field height: expected=\"999\" got=\"0\"") {
		t.Errorf("report missing field mismatch:\n%s", stdout.String())
	}
}

func TestRun_CompareRejectsUnparsableQueryID(t *testing.T) {
	expectedPath := writeFile(t, "bogus-id\tresult=true\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-geodata", t.TempDir(), "-allow-empty-geodata", "-expected-dump", expectedPath}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("exit = %d, want %d; stderr=%s", code, exitError, stderr.String())
	}
}

func TestRun_DumpFlagWritesToFile(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "out.txt")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-geodata", t.TempDir(), "-allow-empty-geodata", "-queries", "3", "-dump", outPath}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty when -dump is given", stdout.String())
	}
	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read dump file: %v", err)
	}
	if len(strings.Split(strings.TrimRight(string(content), "\n"), "\n")) != 3 {
		t.Fatalf("dump file content = %q, want 3 lines", content)
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dump.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

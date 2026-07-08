package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_List(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-list"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("run(-list) exit = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	for _, name := range []string{"item", "npc", "classtemplate", "playerlevels"} {
		if !strings.Contains(stdout.String(), name) {
			t.Errorf("-list output missing category %q:\n%s", name, stdout.String())
		}
	}
}

func TestRun_MissingCategory(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-datapack", t.TempDir()}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("exit = %d, want %d", code, exitError)
	}
}

func TestRun_UnknownCategory(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-category", "nope", "-datapack", t.TempDir()}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("exit = %d, want %d", code, exitError)
	}
}

func TestRun_BothSourcesGiven(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-category", "item", "-datapack", t.TempDir(), "-dump", t.TempDir()}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("exit = %d, want %d", code, exitError)
	}
}

// writeDumpFile writes content to a new file under t.TempDir() and returns
// its path.
func writeDumpFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dump.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write dump fixture: %v", err)
	}
	return path
}

func TestRun_DumpModePassesThroughDumpFile(t *testing.T) {
	dumpPath := writeDumpFile(t, "1\tname=a\n2\tname=b\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-category", "item", "-dump", dumpPath}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	if stdout.String() != "1\tname=a\n2\tname=b\n" {
		t.Fatalf("dump output = %q", stdout.String())
	}
}

func TestRun_CompareMatch(t *testing.T) {
	dumpPath := writeDumpFile(t, "1\tname=a\n")
	expectedPath := writeDumpFile(t, "1\tname=a\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-category", "item", "-dump", dumpPath, "-expected-dump", expectedPath}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "no differences") {
		t.Errorf("report = %q, want it to report no differences", stdout.String())
	}
}

func TestRun_CompareMismatch(t *testing.T) {
	dumpPath := writeDumpFile(t, "1\tname=a\n2\tname=z\n")
	expectedPath := writeDumpFile(t, "1\tname=a\n3\tname=c\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-category", "item", "-dump", dumpPath, "-expected-dump", expectedPath}, &stdout, &stderr)
	if code != exitDiffFound {
		t.Fatalf("exit = %d, want %d; stdout=%s stderr=%s", code, exitDiffFound, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "only in expected dump: 3") {
		t.Errorf("report missing only-in-expected line:\n%s", out)
	}
	if !strings.Contains(out, "only in loaded records: 2") {
		t.Errorf("report missing only-in-loaded line:\n%s", out)
	}
}

func TestRun_LoadFromMissingDatapackDirIsAnError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-category", "item", "-datapack", filepath.Join(t.TempDir(), "does-not-exist")}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("exit = %d, want %d", code, exitError)
	}
}

func TestFindDatapackDir_FindsSiblingCheckout(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, "acis_public", "acis_golang")
	if err := os.MkdirAll(module, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "acis_public", "aCis_datapack")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := findDatapackDir(module)
	if !ok {
		t.Fatal("findDatapackDir() = not found, want datapack path")
	}
	if got != want {
		t.Fatalf("findDatapackDir() = %q, want %q", got, want)
	}
}

func TestResolveDatapackDir_AutoDiscoversFromSourceRoot(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, "acis_public", "acis_golang")
	if err := os.MkdirAll(module, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "acis_public", "aCis_datapack")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(root, "elsewhere")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	})

	oldSourceRoot := sourceRoot
	sourceRoot = func() string { return module }
	t.Cleanup(func() { sourceRoot = oldSourceRoot })

	got, err := resolveDatapackDir("")
	if err != nil {
		t.Fatalf("resolveDatapackDir() error: %v", err)
	}
	got, err = filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatal(err)
	}
	want, err = filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("resolveDatapackDir() = %q, want %q", got, want)
	}
}

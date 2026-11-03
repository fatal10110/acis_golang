package datapack

import (
	"os"
	"path/filepath"
	"testing"
)

// helperDir is where this package sits below a Go checkout root.
var helperDir = filepath.Join("internal", "testsupport", "datapack")

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
}

func TestFindFromResolvesWorkspaceLayouts(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "acis_public")
	want := filepath.Join(ws, "aCis_datapack")
	primary := filepath.Join(ws, "acis_golang", helperDir)
	worktree := filepath.Join(ws, "acis_golang-worktrees", "issue-1566", helperDir)
	mkdirs(t, want, primary, worktree)

	for name, start := range map[string]string{"primary checkout": primary, "linked worktree": worktree} {
		got, ok := findFrom(start, "aCis_datapack", "")
		if !ok || got != want {
			t.Errorf("%s: findFrom(%s) = %q, %v; want %q, true", name, start, got, ok, want)
		}
	}
}

func TestFindFromResolvesCheckoutBesideWorkspace(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "acis_public", "aCis_datapack")
	start := filepath.Join(root, "acis_golang", helperDir)
	mkdirs(t, want, start)

	if got, ok := findFrom(start, "aCis_datapack", ""); !ok || got != want {
		t.Fatalf("findFrom(%s) = %q, %v; want %q, true", start, got, ok, want)
	}
}

func TestFindFromIgnoresNonDirectory(t *testing.T) {
	ws := t.TempDir()
	start := filepath.Join(ws, "acis_golang", helperDir)
	mkdirs(t, start)
	if err := os.WriteFile(filepath.Join(ws, "aCis_datapack"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if got, ok := findFrom(start, "aCis_datapack", ""); ok {
		t.Fatalf("findFrom(%s) = %q, true; want a plain file ignored", start, got)
	}
}

// recorder captures how missing ends a test without stopping this one.
type recorder struct {
	testing.TB
	skipped, failed bool
}

func (r *recorder) Helper()               { /* no-op: records only */ }
func (r *recorder) Skipf(string, ...any)  { r.skipped = true }
func (r *recorder) Fatalf(string, ...any) { r.failed = true }

func TestMissingSkipsByDefaultAndFailsWhenRequired(t *testing.T) {
	t.Setenv(requireEnv, "")
	skip := &recorder{}
	missing(skip)
	if !skip.skipped || skip.failed {
		t.Fatalf("without %s: skipped=%v failed=%v, want skip", requireEnv, skip.skipped, skip.failed)
	}

	t.Setenv(requireEnv, "1")
	fail := &recorder{}
	missing(fail)
	if !fail.failed || fail.skipped {
		t.Fatalf("with %s=1: skipped=%v failed=%v, want failure", requireEnv, fail.skipped, fail.failed)
	}
}

// TestRequireFindsSharedDatapack resolves the real checkout from wherever
// this module sits: it passes from the primary checkout and from a linked
// worktree, and fails under ACIS_REQUIRE_DATAPACK when neither can see it.
func TestRequireFindsSharedDatapack(t *testing.T) {
	skills := Path(t, "data", "xml", "skills")
	if info, err := os.Stat(skills); err != nil || !info.IsDir() {
		t.Fatalf("resolved datapack has no skills directory at %s: %v", skills, err)
	}
}

func TestFindFromSkipsDatapackWithoutSub(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "acis_public")
	start := filepath.Join(ws, "acis_golang", helperDir)
	inRepo := filepath.Join(ws, "acis_golang", "aCis_datapack")
	full := filepath.Join(ws, "aCis_datapack")
	mkdirs(t, start, filepath.Join(inRepo, "data", "xml"), filepath.Join(full, geodataDir))

	if got, ok := findFrom(start, "aCis_datapack", ""); !ok || got != inRepo {
		t.Errorf("findFrom(%s, \"\") = %q, %v; want the in-repo copy %q", start, got, ok, inRepo)
	}
	if got, ok := findFrom(start, "aCis_datapack", geodataDir); !ok || got != full {
		t.Errorf("findFrom(%s, %s) = %q, %v; want the full checkout %q", start, geodataDir, got, ok, full)
	}
}

func TestGeodataSkipsWhenAbsentEvenWhenRequired(t *testing.T) {
	t.Setenv(requireEnv, "1")
	t.Setenv(geodataEnv, "")
	root := filepath.Join(t.TempDir(), "acis_golang")
	start := filepath.Join(root, helperDir)
	mkdirs(t, start, filepath.Join(root, "aCis_datapack", "data", "xml"))

	r := &recorder{}
	geodata(r, start)
	if !r.skipped || r.failed {
		t.Fatalf("geodata without data/geodata: skipped=%v failed=%v, want skip", r.skipped, r.failed)
	}
}

func TestGeodataPrefersEnv(t *testing.T) {
	want := t.TempDir()
	t.Setenv(geodataEnv, want)
	r := &recorder{}
	if got := geodata(r, t.TempDir()); got != want || r.skipped || r.failed {
		t.Fatalf("geodata() = %q skipped=%v failed=%v; want %q", got, r.skipped, r.failed, want)
	}
}

func TestReferenceSkipsOrFailsByEnv(t *testing.T) {
	start := filepath.Join(t.TempDir(), "acis_golang", helperDir)
	mkdirs(t, start)
	for _, c := range []struct {
		require, reference string
		wantFail           bool
	}{
		{"", "", false},
		{"1", "", true},
		{"1", "0", false},
		{"", "1", true},
	} {
		t.Setenv(requireEnv, c.require)
		t.Setenv(referenceEnv, c.reference)
		r := &recorder{}
		reference(r, start, "java")
		if r.failed != c.wantFail || r.skipped == c.wantFail {
			t.Errorf("%s=%q %s=%q: skipped=%v failed=%v, want failed=%v", requireEnv, c.require, referenceEnv, c.reference, r.skipped, r.failed, c.wantFail)
		}
	}
}

func TestReferenceSkipsCheckoutWithoutSub(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "acis_public")
	start := filepath.Join(ws, "acis_golang-worktrees", "task", helperDir)
	want := filepath.Join(ws, "aCis_gameserver")
	mkdirs(t, start, filepath.Join(ws, "acis_golang-worktrees", "aCis_gameserver", "java"), filepath.Join(want, "java", "net"))

	r := &recorder{}
	if got := reference(r, start, filepath.Join("java", "net")); got != want || r.skipped || r.failed {
		t.Fatalf("reference() = %q skipped=%v failed=%v; want %q", got, r.skipped, r.failed, want)
	}
}

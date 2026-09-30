// Package datapack locates the shared aCis_datapack checkout for tests that
// compare loaders and gameplay against the shipped data files.
package datapack

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// requireEnv names the environment variable that turns a missing datapack
// from a skip into a test failure. Set it to any non-empty value when the
// run is a completion gate, so an absent checkout cannot pass as coverage.
const requireEnv = "ACIS_REQUIRE_DATAPACK"

// Find returns the aCis_datapack directory, or false when none is checked
// out. It searches every ancestor of this module, so the primary checkout
// (acis_public/acis_golang) and a linked worktree at any depth below the
// workspace (acis_public/acis_golang-worktrees/<task>) resolve the same
// directory. Resolution is relative to this source file, not the working
// directory `go test` runs in.
func Find() (string, bool) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	return findFrom(filepath.Dir(thisFile))
}

// findFrom walks from start to the filesystem root and returns the first
// aCis_datapack directory found either directly under an ancestor or under
// an ancestor's acis_public directory.
func findFrom(start string) (string, bool) {
	for dir := filepath.Clean(start); ; {
		for _, candidate := range []string{
			filepath.Join(dir, "aCis_datapack"),
			filepath.Join(dir, "acis_public", "aCis_datapack"),
		} {
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// Require returns the aCis_datapack directory. When none is checked out it
// skips the calling test, or fails it when requireEnv is set.
func Require(t testing.TB) string {
	t.Helper()
	dir, ok := Find()
	if !ok {
		missing(t)
	}
	return dir
}

// Path returns elem joined under the aCis_datapack directory, ending the
// calling test as Require does when the datapack is not checked out.
func Path(t testing.TB, elem ...string) string {
	t.Helper()
	return filepath.Join(append([]string{Require(t)}, elem...)...)
}

func missing(t testing.TB) {
	t.Helper()
	if os.Getenv(requireEnv) != "" {
		t.Fatalf("aCis_datapack not found above the module root and %s is set", requireEnv)
		return
	}
	t.Skipf("aCis_datapack not checked out near the module root, skipping oracle comparison (set %s=1 to fail instead)", requireEnv)
}

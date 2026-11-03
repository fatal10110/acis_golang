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

// referenceEnv names the environment variable that decides whether a missing
// reference server checkout fails a test: "0" skips, any other non-empty
// value fails. Unset, it follows requireEnv. CI sets it to "0": it has the
// repository's datapack copy but not the reference checkout.
const referenceEnv = "ACIS_REQUIRE_REFERENCE"

// geodataEnv names the environment variable that points the geodata tests at
// a data/geodata directory directly.
const geodataEnv = "ACIS_GEODATA_DIR"

// geodataDir is where a datapack keeps its region files. The repository's
// own copy of the datapack leaves it out (it is far too large), so only a
// full checkout beside the module provides it.
var geodataDir = filepath.Join("data", "geodata")

// Find returns the aCis_datapack directory, or false when none is checked
// out. It searches the module root and every ancestor, so the repository's
// own copy wins over a full checkout further up. Resolution is relative to
// this source file, not the working directory `go test` runs in.
func Find() (string, bool) {
	start, ok := sourceDir()
	if !ok {
		return "", false
	}
	return findFrom(start, "aCis_datapack", "")
}

func sourceDir() (string, bool) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	return filepath.Dir(thisFile), true
}

// findFrom walks from start to the filesystem root and returns the first
// directory called name, either directly under an ancestor or under an
// ancestor's acis_public directory, that holds the path sub ("" for the
// directory itself).
func findFrom(start, name, sub string) (string, bool) {
	for dir := filepath.Clean(start); ; {
		for _, candidate := range []string{
			filepath.Join(dir, name),
			filepath.Join(dir, "acis_public", name),
		} {
			if info, err := os.Stat(filepath.Join(candidate, sub)); err == nil && (sub != "" || info.IsDir()) {
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

// Geodata returns the geodata directory: $ACIS_GEODATA_DIR when set, else
// data/geodata of the nearest datapack that has one. Without one it skips
// the calling test even when requireEnv is set, because the repository's
// datapack copy carries no geodata and CI has none.
func Geodata(t testing.TB) string {
	t.Helper()
	start, _ := sourceDir()
	return geodata(t, start)
}

func geodata(t testing.TB, start string) string {
	t.Helper()
	if dir := os.Getenv(geodataEnv); dir != "" {
		return dir
	}
	dir, ok := findFrom(start, "aCis_datapack", geodataDir)
	if !ok {
		t.Skipf("no aCis_datapack with %s found above the module root and %s unset; geodata is not in the repository", geodataDir, geodataEnv)
		return ""
	}
	return filepath.Join(dir, geodataDir)
}

// Reference returns the reference server checkout (aCis_gameserver) that
// holds the path sub, found above the module as Find finds the datapack.
// Without one it skips the calling test, or fails it when referenceEnv (or,
// with referenceEnv unset, requireEnv) asks for the reference.
func Reference(t testing.TB, sub string) string {
	t.Helper()
	start, _ := sourceDir()
	return reference(t, start, sub)
}

func reference(t testing.TB, start, sub string) string {
	t.Helper()
	dir, ok := findFrom(start, "aCis_gameserver", sub)
	if ok {
		return dir
	}
	required := os.Getenv(requireEnv) != ""
	if v := os.Getenv(referenceEnv); v != "" {
		required = v != "0"
	}
	if required {
		t.Fatalf("no aCis_gameserver with %s found above the module root (%s or %s asks for it)", sub, referenceEnv, requireEnv)
		return ""
	}
	t.Skipf("no aCis_gameserver with %s found above the module root, skipping reference comparison (set %s=1 to fail instead)", sub, referenceEnv)
	return ""
}

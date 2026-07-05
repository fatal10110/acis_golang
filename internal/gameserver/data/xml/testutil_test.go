package xml

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// datapackPath resolves rel under the shared aCis_datapack checkout that
// sits next to the module root (the same files the Java oracle loads), and
// skips the calling test when the checkout is absent. Resolution is
// relative to this source file so it works regardless of the directory
// `go test` is invoked from.
func datapackPath(t *testing.T, rel string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed to resolve test file path")
	}
	// this file lives at <workspace>/<checkout>/internal/gameserver/data/xml
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..", "aCis_datapack", rel)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("aCis_datapack not checked out next to the module root, skipping oracle comparison: %v", err)
	}
	return path
}

// writeXMLFixture writes a small XML fixture for parser error-path tests.
func writeXMLFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test fixture %q: %v", path, err)
	}
}

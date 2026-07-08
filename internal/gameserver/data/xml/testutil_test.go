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
	// this file lives at <checkout>/internal/gameserver/data/xml
	checkout := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	candidates := []string{
		filepath.Join(checkout, "..", "aCis_datapack", rel),
		filepath.Join(checkout, "..", "..", "acis_public", "aCis_datapack", rel),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	t.Skipf("aCis_datapack not checked out near the module root, skipping oracle comparison")
	return ""
}

// writeXMLFixture writes a small XML fixture for parser error-path tests.
func writeXMLFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test fixture %q: %v", path, err)
	}
}

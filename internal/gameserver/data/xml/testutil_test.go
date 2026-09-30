package xml

import (
	"os"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// datapackPath resolves rel under the shared aCis_datapack checkout (the
// same files the reference server loads), skipping the calling test when the
// checkout is absent.
func datapackPath(t *testing.T, rel string) string {
	t.Helper()
	return datapack.Path(t, rel)
}

// writeXMLFixture writes a small XML fixture for parser error-path tests.
func writeXMLFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test fixture %q: %v", path, err)
	}
}

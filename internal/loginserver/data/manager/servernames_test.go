package manager

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadServerNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "serverNames.xml")
	mustWriteFile(t, path, `<?xml version='1.0' encoding='utf-8'?>
<list>
	<!-- comment -->
	<server id="1" name="Bartz" />
	<server id="5" name="Erica" />
</list>`)

	names, err := LoadServerNames(path)
	if err != nil {
		t.Fatalf("LoadServerNames: %v", err)
	}

	if got, ok := names.Name(1); !ok || got != "Bartz" {
		t.Fatalf("Name(1) = %q, %v", got, ok)
	}
	if got, ok := names.Name(5); !ok || got != "Erica" {
		t.Fatalf("Name(5) = %q, %v", got, ok)
	}
	if _, ok := names.Name(2); ok {
		t.Fatal("Name(2) = true, want false")
	}
	if want := []int{1, 5}; !reflect.DeepEqual(names.IDs(), want) {
		t.Fatalf("IDs() = %v, want %v", names.IDs(), want)
	}
}

func TestLoadServerNamesMissingFile(t *testing.T) {
	if _, err := LoadServerNames(filepath.Join(t.TempDir(), "missing.xml")); err == nil {
		t.Fatal("LoadServerNames() error = nil, want error for missing file")
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

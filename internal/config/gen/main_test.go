package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
)

// TestSupportedKeysUpToDate rebuilds the registry from the actual readers
// and reference .properties files and fails if it differs from the
// committed supported_keys_generated.go, so the registry can't silently
// drift from either side without `go generate` catching it.
func TestSupportedKeysUpToDate(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	goRoot := filepath.Join(cwd, "..", "..", "..") // gen -> config -> internal -> <go-root>, since `go test` runs in this package's own dir
	refConfigDir := filepath.Join(cwd, "..", "testdata", "reference")

	readKeys, err := scanReadKeys(goRoot)
	if err != nil {
		t.Fatal(err)
	}

	got := make(map[string][]string, len(config.ShippedFiles))
	for _, name := range config.ShippedFiles {
		props, err := config.LoadFile(filepath.Join(refConfigDir, name))
		if err != nil {
			t.Fatal(err)
		}
		keys := []string{}
		for _, key := range props.Keys() {
			if readKeys[key] {
				keys = append(keys, key)
			}
		}
		got[name] = keys
	}

	if !reflect.DeepEqual(got, config.SupportedKeys) {
		t.Fatalf("config.SupportedKeys is stale; run `go generate ./internal/config/...`\nwant %#v\ngot  %#v", got, config.SupportedKeys)
	}
}

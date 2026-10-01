package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/rs/zerolog"
)

// A crest load that stops at a bad file keeps the crests read before it and
// boots on, but says so: an error naming the file, a warning per deleted
// invalid crest, and the loaded count.
func TestLoadCrestCacheReportsPartialLoad(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "data", "crests")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	valid := bytes.Repeat([]byte{0x5a}, 256)
	for name, data := range map[string][]byte{
		"Crest_101.dds":      valid,
		"Crest_102.dds":      {0x01},
		"Crest_bad.dds":      valid,
		"LargeCrest_103.dds": bytes.Repeat([]byte{0x5b}, 2176),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	crests, err := loadCrestCache(gameServerPaths{DataRoot: root}, zerolog.New(&out))
	if err != nil {
		t.Fatalf("loadCrestCache: %v, want the partial cache and no boot failure", err)
	}
	if _, ok := crests.Get(datacache.PledgeCrest, 101); !ok || crests.Len() != 1 {
		t.Fatalf("cache holds %d crests (Crest_101 %v), want only Crest_101", crests.Len(), ok)
	}

	logs := out.String()
	for _, want := range []string{
		`"level":"error"`, "Crest_bad.dds",
		`"level":"warn"`, `"crest":"Crest_102.dds"`,
		`"crests":1`,
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("crest load log lacks %s:\n%s", want, logs)
		}
	}
}

// A missing crest directory is logged at error level, as the reference
// does, and leaves an empty cache.
func TestLoadCrestCacheReportsMissingDirectory(t *testing.T) {
	var out bytes.Buffer
	crests, err := loadCrestCache(gameServerPaths{DataRoot: t.TempDir()}, zerolog.New(&out))
	if err != nil || crests.Len() != 0 {
		t.Fatalf("loadCrestCache = %d crests, %v; want an empty cache", crests.Len(), err)
	}
	if logs := out.String(); !strings.Contains(logs, `"level":"error"`) || !strings.Contains(logs, `"crests":0`) {
		t.Fatalf("crest load log lacks the error and count:\n%s", logs)
	}
}

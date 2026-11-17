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

// A crest load skips a bad file, loads every other crest and boots on, but
// says so: an error naming the file, a warning per deleted invalid crest,
// and the loaded count.
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
	_, ok101 := crests.Get(datacache.PledgeCrest, 101)
	_, ok103 := crests.Get(datacache.LargePledgeCrest, 103)
	if !ok101 || !ok103 || crests.Len() != 2 {
		t.Fatalf("cache holds %d crests (Crest_101 %v, LargeCrest_103 %v), want both", crests.Len(), ok101, ok103)
	}

	logs := out.String()
	for _, want := range []string{
		`"level":"error"`, "Crest_bad.dds",
		`"level":"warn"`, `"crest":"Crest_102.dds"`,
		`"crests":2`,
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

// The //reload crest hook serves what the directory holds after the reload,
// including a crest uploaded since boot, and skips a bad file as the boot
// does.
func TestCrestReloadHookServesUploadedCrest(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "data", "crests")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	valid := bytes.Repeat([]byte{0x5a}, 256)
	if err := os.WriteFile(filepath.Join(dir, "Crest_101.dds"), valid, 0o600); err != nil {
		t.Fatal(err)
	}
	paths := gameServerPaths{DataRoot: root}
	crests, err := loadCrestCache(paths, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if err := crests.Save(datacache.PledgeCrest, 102, valid); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Crest_bad.dds"), valid, 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	reloads := provideDataReloads(paths, gameServerConfig{}, nil, nil, crests, nil, nil, gameplayConfig{}, zerolog.New(&out))
	if err := reloads.Crests(); err != nil {
		t.Fatalf("crest reload: %v", err)
	}
	for _, id := range []int{101, 102} {
		if _, ok := crests.Get(datacache.PledgeCrest, id); !ok {
			t.Fatalf("Crest_%d not served after the reload", id)
		}
	}
	if logs := out.String(); !strings.Contains(logs, "Crest_bad.dds") || !strings.Contains(logs, `"crests":2`) {
		t.Fatalf("crest reload log lacks the skipped file and count:\n%s", logs)
	}
}

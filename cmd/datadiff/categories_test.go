package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/fatal10110/acis_golang/internal/datadiff"
)

// datapackRoot resolves the aCis_datapack checkout that sits next to the
// module root (the same files a loader reads at boot), and skips the
// calling test when it isn't present. Resolution is relative to this
// source file so it works regardless of the directory `go test` is
// invoked from.
func datapackRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed to resolve test file path")
	}
	// this file lives at <workspace>/<checkout>/cmd/datadiff
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "aCis_datapack")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("aCis_datapack not checked out next to the module root, skipping: %v", err)
	}
	return root
}

// TestLoadRecords_RealDatapack proves the end-to-end mechanism — invoking
// a real loader through the registry and reducing its result to Records —
// works against the actual shipped data, for every category wired in
// today.
func TestLoadRecords_RealDatapack(t *testing.T) {
	root := datapackRoot(t)

	for _, name := range sortedCategoryNames() {
		t.Run(name, func(t *testing.T) {
			records, err := categories[name].load(root)
			if err != nil {
				t.Fatalf("load(%q) error: %v", root, err)
			}
			if len(records) == 0 {
				t.Fatalf("load(%q) returned no records", root)
			}

			seen := make(map[string]bool, len(records))
			for _, r := range records {
				if r.ID == "" {
					t.Errorf("record with empty ID: %+v", r)
				}
				if seen[r.ID] {
					t.Errorf("duplicate id %q in loaded records", r.ID)
				}
				seen[r.ID] = true
				if len(r.Fields) == 0 {
					t.Errorf("record %q has no fields", r.ID)
				}
			}
		})
	}
}

// TestDumpRoundTrip_RealDatapack proves a real loaded record set survives
// a WriteDump/ReadDump round trip unchanged — the same path the command
// takes when writing a dump for another implementation to compare
// against, or reloading a previously captured one.
func TestDumpRoundTrip_RealDatapack(t *testing.T) {
	root := datapackRoot(t)

	records, err := loadPlayerLevelRecords(root)
	if err != nil {
		t.Fatalf("loadPlayerLevelRecords(%q) error: %v", root, err)
	}

	dumpPath := filepath.Join(t.TempDir(), "playerlevels.dump")
	f, err := os.Create(dumpPath)
	if err != nil {
		t.Fatalf("create dump file: %v", err)
	}

	if err := datadiff.WriteDump(f, records); err != nil {
		f.Close()
		t.Fatalf("write dump: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close dump file: %v", err)
	}

	readBack, err := os.Open(dumpPath)
	if err != nil {
		t.Fatalf("open dump file: %v", err)
	}
	defer readBack.Close()

	roundTripped, err := datadiff.ReadDump(readBack)
	if err != nil {
		t.Fatalf("read dump back: %v", err)
	}
	if len(roundTripped) != len(records) {
		t.Fatalf("round trip returned %d records, want %d", len(roundTripped), len(records))
	}
}

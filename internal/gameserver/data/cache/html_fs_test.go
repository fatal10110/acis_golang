package cache

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// TestLoadHTMLFSMatchesLoadHTML: the same pages load identically from a
// directory and from an in-memory file system: keys, normalized line ends,
// undecodable pages left out, non-page files ignored.
func TestLoadHTMLFSMatchesLoadHTML(t *testing.T) {
	pages := map[string]string{
		"a/one.htm":     "line1\r\nline2",
		"b/two.HTML":    "x\ry",
		"c/latin1.htm":  "caf\xe9",
		"c/notes.txt":   "not a page",
		"data/html.htm": "<html/>",
	}
	files := fstest.MapFS{}
	dir := t.TempDir()
	for name, content := range pages {
		files[name] = &fstest.MapFile{Data: []byte(content)}
		writeHTML(t, dir, name, content)
	}

	fromDisk, diskBad, err := LoadHTML(dir)
	if err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	fromFS, fsBad, err := LoadHTMLFS(files)
	if err != nil {
		t.Fatalf("LoadHTMLFS: %v", err)
	}
	if !slices.Equal(diskBad, fsBad) || !slices.Equal(fromDisk.Paths(), fromFS.Paths()) {
		t.Fatalf("pages differ: disk %q %q, fs %q %q", fromDisk.Paths(), diskBad, fromFS.Paths(), fsBad)
	}
	for name := range pages {
		want, wantOK := fromDisk.Get(name)
		got, ok := fromFS.Get(name)
		if got != want || ok != wantOK {
			t.Errorf("Get(%q) = %q, %t; from disk %q, %t", name, got, ok, want, wantOK)
		}
	}
	if got, _ := fromFS.Get("data/html/a/one.htm"); got != "line1\nline2\n" {
		t.Errorf("Get with the data/html prefix = %q", got)
	}
}

func TestLoadHTMLNoPagesNamesTheDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadHTML(dir); err == nil || !strings.Contains(err.Error(), dir) {
		t.Fatalf("LoadHTML error = %v, want it to name %s", err, dir)
	}
	if _, _, err := LoadHTMLFS(fstest.MapFS{}); err == nil {
		t.Fatal("LoadHTMLFS of an empty file system succeeded")
	}
}

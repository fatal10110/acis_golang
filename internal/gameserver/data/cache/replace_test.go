package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func writeFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Replace is HtmCache.reload: the pages read anew reach every holder of the
// cache, and a read beside it sees the old pages or the new ones.
func TestHTMLReplace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "admin/main.htm", []byte("<html>old</html>"))
	writeFile(t, dir, "admin/gone.htm", []byte("<html>gone</html>"))
	html, err := LoadHTML(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "admin/main.htm", []byte("<html>new</html>"))
	if err := os.Remove(filepath.Join(dir, "admin/gone.htm")); err != nil {
		t.Fatal(err)
	}
	fresh, err := LoadHTML(dir)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 1000 {
			if page, ok := html.Get("data/html/admin/main.htm"); !ok || page != "<html>old</html>\n" && page != "<html>new</html>\n" {
				t.Errorf("Get(main) = %q, %v", page, ok)
				return
			}
			html.Paths()
		}
	}()
	html.Replace(fresh)
	wg.Wait()

	if page, _ := html.Get("admin/main.htm"); page != "<html>new</html>\n" {
		t.Fatalf("main page after the replace = %q, want the new one", page)
	}
	if _, ok := html.Get("admin/gone.htm"); ok || html.Len() != 1 {
		t.Fatalf("a page deleted from disk is still served (%d pages)", html.Len())
	}
}

// Replace is CrestCache.reload: the cache holds what the directory holds
// now, keeps saving there, and a crest request beside it is race-free.
func TestCrestsReplace(t *testing.T) {
	dir := t.TempDir()
	old := bytes.Repeat([]byte{1}, 256)
	writeFile(t, dir, "Crest_1.dds", old)
	crests, _, err := LoadCrests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "Crest_1.dds")); err != nil {
		t.Fatal(err)
	}
	ally := bytes.Repeat([]byte{2}, 192)
	writeFile(t, dir, "AllyCrest_2.dds", ally)
	fresh, _, err := LoadCrests(dir)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 1000 {
			crests.Get(PledgeCrest, 1)
			crests.Has(AllyCrest, 2)
		}
	}()
	crests.Replace(fresh)
	wg.Wait()

	if crests.Has(PledgeCrest, 1) {
		t.Fatal("a crest deleted from disk is still served")
	}
	if got, ok := crests.Get(AllyCrest, 2); !ok || !bytes.Equal(got, ally) {
		t.Fatal("the alliance crest on disk is not served")
	}
	if err := crests.Save(PledgeCrest, 3, old); err != nil {
		t.Fatalf("Save() after the replace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Crest_3.dds")); err != nil {
		t.Fatalf("saved crest not on disk: %v", err)
	}
}

package cache

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// A page is read from disk on its first request, as the HtmCache does, and
// served from memory after that: loading the directory keeps no page
// content, only which pages there are.
func TestHTMLReadsPageOnFirstRequest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "default/30001.htm", []byte("<html>boot</html>"))
	writeFile(t, dir, "default/30002.htm", []byte("<html>other</html>"))
	html, _, err := LoadHTML(dir)
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, dir, "default/30001.htm", []byte("<html>first request</html>\r\n"))
	if page, ok := html.Get("default/30001.htm"); !ok || page != "<html>first request</html>\n" {
		t.Fatalf("first request = %q, %v; want the page as on disk then", page, ok)
	}
	writeFile(t, dir, "default/30001.htm", []byte("<html>later</html>"))
	if page, _ := html.Get("data/html/default/30001.htm"); page != "<html>first request</html>\n" {
		t.Fatalf("second request = %q; want the page read on the first one", page)
	}

	if err := os.Remove(filepath.Join(dir, "default/30002.htm")); err != nil {
		t.Fatal(err)
	}
	if page, ok := html.Get("default/30002.htm"); ok {
		t.Fatalf("a page gone from disk before its first request = %q, want missing", page)
	}
	if html.Len() != 2 || len(html.Paths()) != 2 {
		t.Fatalf("Len = %d, Paths = %v; want the 2 pages the load found", html.Len(), html.Paths())
	}
}

// A page that is no longer valid UTF-8 at its first request reads as
// missing, like one that was not at load, and is not kept: it is read again
// on the next request.
func TestHTMLFirstRequestUndecodable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.htm", []byte("<html>ok</html>"))
	html, _, err := LoadHTML(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "a.htm", []byte{'<', 0xff, '>'})
	if page, ok := html.Get("a.htm"); ok {
		t.Fatalf("Get = %q, want missing", page)
	}
	writeFile(t, dir, "a.htm", []byte("<html>fixed</html>\n"))
	if page, ok := html.Get("a.htm"); !ok || page != "<html>fixed</html>\n" {
		t.Fatalf("Get after the fix = %q, %v", page, ok)
	}
}

// A file added after the load is no page until the next load: a request
// never reaches the disk for a name the load did not find.
func TestHTMLServesOnlyLoadedPages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.htm", []byte("<html>a</html>"))
	html, _, err := LoadHTML(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "b.htm", []byte("<html>b</html>"))
	if page, ok := html.Get("b.htm"); ok {
		t.Fatalf("Get(new file) = %q, want missing", page)
	}
}

// Concurrent first requests for one page all get the same content.
func TestHTMLConcurrentFirstRequests(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.htm", []byte("<html>a</html>"))
	html, _, err := LoadHTML(dir)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			if page, ok := html.Get("a.htm"); !ok || page != "<html>a</html>\n" {
				t.Errorf("Get = %q, %v", page, ok)
			}
		})
	}
	wg.Wait()
}

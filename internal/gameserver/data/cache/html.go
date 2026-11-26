package cache

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

// HTML stores the datapack HTML pages, keyed by path relative to data/html.
// A load records which pages there are; each page is read from disk on its
// first request and kept for later ones, so only requested pages take
// memory. It is safe for concurrent use after LoadHTML returns; //reload htm
// replaces the pages in place with Replace, and each read sees the old
// pages or the new ones.
type HTML struct {
	pages atomic.Pointer[htmlPages]
}

// htmlPages is one load of the HTML directory.
type htmlPages struct {
	fsys fs.FS
	// names are the pages the load found, sorted.
	names []string
	// read holds, per page name, the content of each page read so far.
	read sync.Map
}

// LoadHTML lists every .htm and .html file under dir as a page, reading
// each once to check it. A file that is not valid UTF-8 cannot be read as a
// page: it is left out, so Get reports it missing, and its path relative to
// dir is returned in undecodable, sorted, for the caller to report.
func LoadHTML(dir string) (h *HTML, undecodable []string, err error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("data/cache: stat html dir %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("data/cache: html path %s is not a directory", dir)
	}
	h, undecodable, err = LoadHTMLFS(os.DirFS(dir))
	if errors.Is(err, errNoHTML) {
		return nil, nil, fmt.Errorf("data/cache: no html files found in %s", dir)
	}
	return h, undecodable, err
}

// errNoHTML reports a file system that holds no page.
var errNoHTML = errors.New("no html files found")

// LoadHTMLFS is LoadHTML for the pages under the root of fsys, such as an
// in-memory file system.
func LoadHTMLFS(fsys fs.FS) (h *HTML, undecodable []string, err error) {
	var names []string
	if err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("data/cache: walk html %s: %w", name, err)
		}
		ext := path.Ext(entry.Name())
		if entry.IsDir() || !entry.Type().IsRegular() || (!strings.EqualFold(ext, ".htm") && !strings.EqualFold(ext, ".html")) {
			return nil
		}

		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("data/cache: read html %s: %w", name, err)
		}
		key := path.Clean(name)
		if !utf8.Valid(data) {
			undecodable = append(undecodable, key)
			return nil
		}
		names = append(names, key)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	if len(names) == 0 {
		return nil, nil, errNoHTML
	}
	sort.Strings(names)
	sort.Strings(undecodable)

	h = &HTML{}
	h.pages.Store(&htmlPages{fsys: fsys, names: packNames(names)})
	return h, undecodable, nil
}

// packNames returns names in an exactly sized slice whose strings share one
// buffer, so the page list the cache keeps is two allocations.
func packNames(names []string) []string {
	var buf strings.Builder
	for _, name := range names {
		buf.WriteString(name)
	}
	all := buf.String()
	packed := make([]string, len(names))
	for i, name := range names {
		packed[i], all = all[:len(name)], all[len(name):]
	}
	return packed
}

// Replace swaps h's pages for from's, at once for every holder of h, as an
// HTML reload does: every page is read from disk again.
func (h *HTML) Replace(from *HTML) {
	h.pages.Store(from.pages.Load())
}

// loaded returns the current pages; a nil cache has none.
func (h *HTML) loaded() *htmlPages {
	if h == nil {
		return nil
	}
	return h.pages.Load()
}

// get returns the page named key, reading it from disk on its first
// request. A name the load did not find is no page, without a disk read. A
// page that cannot be read, or is no longer valid UTF-8, is missing and is
// read again on the next request.
func (p *htmlPages) get(key string) (string, bool) {
	if p == nil {
		return "", false
	}
	if content, ok := p.read.Load(key); ok {
		return content.(string), true
	}
	if _, found := slices.BinarySearch(p.names, key); !found {
		return "", false
	}
	data, err := fs.ReadFile(p.fsys, key)
	if err != nil || !utf8.Valid(data) {
		return "", false
	}
	content, _ := p.read.LoadOrStore(key, normalizeHTML(string(data)))
	return content.(string), true
}

func normalizeHTML(content string) string {
	content = strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content
}

// Get returns the loaded HTML content for name. name may be relative to
// data/html, or prefixed with data/html/.
func (h *HTML) Get(name string) (string, bool) {
	key := htmlKey(name)
	if key == "" {
		return "", false
	}
	return h.loaded().get(key)
}

// Len returns the number of loaded pages.
func (h *HTML) Len() int {
	if p := h.loaded(); p != nil {
		return len(p.names)
	}
	return 0
}

// Paths returns loaded page paths sorted lexically.
func (h *HTML) Paths() []string {
	if p := h.loaded(); p != nil {
		return slices.Clone(p.names)
	}
	return nil
}

// htmlKey is the cache key of name, "" for none. A name with a ".."
// segment names no page: it is refused rather than resolved to the page it
// climbs to.
func htmlKey(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return ""
		}
	}
	key := path.Clean(name)
	if key == "." {
		return ""
	}
	key = strings.TrimPrefix(key, "./")
	return strings.TrimPrefix(key, "data/html/")
}

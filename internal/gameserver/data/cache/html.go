package cache

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
)

// HTML stores loaded datapack HTML pages, keyed by path relative to data/html.
// It is safe for concurrent reads after LoadHTML returns; //reload htm
// replaces the pages in place with Replace, and each read sees the old
// pages or the new ones.
type HTML struct {
	pages atomic.Pointer[map[string]string]
}

// LoadHTML reads every .htm and .html file under dir into memory.
func LoadHTML(dir string) (*HTML, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("data/cache: stat html dir %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("data/cache: html path %s is not a directory", dir)
	}

	pages := make(map[string]string)
	if err := filepath.WalkDir(dir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("data/cache: walk html %s: %w", name, err)
		}
		ext := filepath.Ext(entry.Name())
		if entry.IsDir() || !entry.Type().IsRegular() || (!strings.EqualFold(ext, ".htm") && !strings.EqualFold(ext, ".html")) {
			return nil
		}

		data, err := os.ReadFile(name)
		if err != nil {
			return fmt.Errorf("data/cache: read html %s: %w", name, err)
		}
		rel, err := filepath.Rel(dir, name)
		if err != nil {
			return fmt.Errorf("data/cache: html path %s relative to %s: %w", name, dir, err)
		}
		key := path.Clean(filepath.ToSlash(rel))
		pages[key] = normalizeHTML(string(data))
		return nil
	}); err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("data/cache: no html files found in %s", dir)
	}

	h := &HTML{}
	h.pages.Store(&pages)
	return h, nil
}

// Replace swaps h's pages for from's, at once for every holder of h:
// HtmCache.reload, which the reference runs by emptying its cache so every
// page is read from disk again.
func (h *HTML) Replace(from *HTML) {
	h.pages.Store(from.pages.Load())
}

// loaded returns the current pages; a nil cache has none.
func (h *HTML) loaded() map[string]string {
	if h == nil {
		return nil
	}
	return *h.pages.Load()
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
	content, ok := h.loaded()[htmlKey(name)]
	return content, ok
}

// Len returns the number of loaded pages.
func (h *HTML) Len() int {
	return len(h.loaded())
}

// Paths returns loaded page paths sorted lexically.
func (h *HTML) Paths() []string {
	pages := h.loaded()
	if pages == nil {
		return nil
	}
	paths := make([]string, 0, len(pages))
	for name := range pages {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	return paths
}

func htmlKey(name string) string {
	key := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	if key == "." {
		return ""
	}
	key = strings.TrimPrefix(key, "./")
	return strings.TrimPrefix(key, "data/html/")
}

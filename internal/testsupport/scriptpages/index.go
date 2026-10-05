// Package scriptpages holds the committed index of the datapack's script
// pages (data/html/script/{quest,ai,feature,teleport,siegablehall}) and the
// check that every page literal of a Go script package names a page in it.
//
// The index records, per page, its path, a content hash, the bypass commands
// a client may send back from it and its %placeholder% names. It carries no
// page text. Regenerate it from an unmodified datapack with
//
//	go run ./cmd/pageindex -datapack <aCis_datapack> -o internal/testsupport/scriptpages/index.jsonl
package scriptpages

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	_ "embed" // index.jsonl
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Root is the page tree the index covers, relative to the datapack root.
// Page names in the index are relative to it.
const Root = "data/html/script"

// Page is one indexed script page.
type Page struct {
	// Name is the page path under Root, '/'-separated.
	Name string `json:"page"`
	// Hash is the first 16 hex digits of the SHA-256 of the page as the
	// server holds it: decoded as UTF-8, every line ended by '\n'. A page
	// that is not valid UTF-8 is hashed as its raw file bytes.
	Hash string `json:"hash"`
	// InvalidUTF8 marks a page the server cannot decode, which it treats as
	// missing (#3509). Such a page has no bypasses or placeholders.
	InvalidUTF8 bool `json:"invalidUTF8,omitempty"`
	// Bypass lists, in page order, the bypass commands the page admits as
	// exact matches.
	Bypass []string `json:"bypass,omitempty"`
	// BypassPrefix lists, in page order, the commands of links taking an
	// edit-box value: the text before the link's first '$', admitted as a
	// prefix.
	BypassPrefix []string `json:"bypassPrefix,omitempty"`
	// Placeholders lists the page's distinct %name% tokens, sorted.
	Placeholders []string `json:"placeholders,omitempty"`
}

//go:embed index.jsonl
var committed []byte

// Index maps a page name to its record.
type Index map[string]Page

// Load parses the committed index.
func Load() (Index, error) {
	return Parse(committed)
}

// Parse reads an index in the form Build writes.
func Parse(data []byte) (Index, error) {
	idx := make(Index)
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for line := 1; sc.Scan(); line++ {
		var p Page
		dec := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("scriptpages: index line %d: %w", line, err)
		}
		if _, dup := idx[p.Name]; dup {
			return nil, fmt.Errorf("scriptpages: index line %d: duplicate page %q", line, p.Name)
		}
		idx[p.Name] = p
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scriptpages: read index: %w", err)
	}
	return idx, nil
}

// Build indexes every page under Root of the datapack at datapackDir and
// returns the index bytes: one JSON record per line, sorted by page name.
// The output depends only on the page files, so it is byte-stable.
func Build(datapackDir string) ([]byte, error) {
	root := filepath.Join(datapackDir, filepath.FromSlash(Root))
	var pages []Page
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("scriptpages: walk %s: %w", name, err)
		}
		if !entry.Type().IsRegular() || !isPageFile(entry.Name()) {
			return nil
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return fmt.Errorf("scriptpages: %s relative to %s: %w", name, root, err)
		}
		data, err := readFile(name)
		if err != nil {
			return err
		}
		pages = append(pages, indexPage(path.Clean(filepath.ToSlash(rel)), data))
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("scriptpages: no pages under %s", root)
	}
	slices.SortFunc(pages, func(a, b Page) int { return strings.Compare(a.Name, b.Name) })

	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	for _, p := range pages {
		if err := enc.Encode(p); err != nil {
			return nil, fmt.Errorf("scriptpages: encode %s: %w", p.Name, err)
		}
	}
	return out.Bytes(), nil
}

// isPageFile reports whether a file is loadable as a page: its name ends in
// .htm or .html, in any letter case.
func isPageFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".htm") || strings.HasSuffix(lower, ".html")
}

// indexPage builds the record of the page name whose file holds data.
func indexPage(name string, data []byte) Page {
	if !utf8.Valid(data) {
		return Page{Name: name, Hash: hash(data), InvalidUTF8: true}
	}
	content := held(string(data))
	exact, prefix := bypasses(content)
	return Page{
		Name:         name,
		Hash:         hash([]byte(content)),
		Bypass:       exact,
		BypassPrefix: prefix,
		Placeholders: placeholders(content),
	}
}

func readFile(name string) ([]byte, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("scriptpages: read %s: %w", name, err)
	}
	return data, nil
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// held returns a page's text as the server holds it: read line by line,
// a line ending at '\n', '\r' or "\r\n", and each line ended by '\n'.
func held(text string) string {
	var b strings.Builder
	b.Grow(len(text) + 1)
	for text != "" {
		end := strings.IndexAny(text, "\r\n")
		if end < 0 {
			b.WriteString(text)
			b.WriteByte('\n')
			break
		}
		b.WriteString(text[:end])
		b.WriteByte('\n')
		if text[end] == '\r' && end+1 < len(text) && text[end+1] == '\n' {
			end++
		}
		text = text[end+1:]
	}
	return b.String()
}

// bypasses returns the commands a page admits once sent: each link reads
// "bypass [-h ]<command>" inside double quotes. The three characters after
// "bypass " are skipped when they begin with "-h"; a command holding a '$'
// admits the text before it as a prefix. Positions are UTF-16 units, as the
// server counts them.
func bypasses(content string) (exact, prefix []string) {
	html := utf16.Encode([]rune(content))
	const marker = `"bypass `
	for i := 0; i < len(html); i++ {
		start := indexUnits(html, marker, i)
		if start < 0 {
			break
		}
		finish := indexUnits(html, `"`, start+1)
		if finish < 0 {
			break
		}
		// The server reads the two units after "bypass " and the command
		// span unchecked; where either runs out of bounds it throws, and
		// the page's remaining links are never read.
		if start+10 > len(html) {
			break
		}
		if string(utf16.Decode(html[start+8:start+10])) == "-h" {
			start += 11
		} else {
			start += 8
		}
		i = finish
		if start > finish {
			break
		}
		if dollar := indexUnits(html, "$", start); dollar > 0 && dollar < finish {
			prefix = append(prefix, javaTrim(html[start:dollar]))
			continue
		}
		exact = append(exact, javaTrim(html[start:finish]))
	}
	return exact, prefix
}

// indexUnits returns the first index at or after from where html holds the
// ASCII string sub, or -1.
func indexUnits(html []uint16, sub string, from int) int {
	for i := max(from, 0); i+len(sub) <= len(html); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if html[i+j] != uint16(sub[j]) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// javaTrim drops the units up to and including the space from both ends.
func javaTrim(units []uint16) string {
	for len(units) > 0 && units[0] <= ' ' {
		units = units[1:]
	}
	for len(units) > 0 && units[len(units)-1] <= ' ' {
		units = units[:len(units)-1]
	}
	return string(utf16.Decode(units))
}

var placeholder = regexp.MustCompile(`%[A-Za-z_][A-Za-z0-9_]*%`)

func placeholders(content string) []string {
	found := placeholder.FindAllString(content, -1)
	slices.Sort(found)
	return slices.Compact(found)
}

package scriptpages

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// CheckPackage reports every page literal in the Go package at dir that
// names no page of idx. pageDir is the script's page directory under Root,
// such as "quest/Q001_LettersOfLove".
//
// A page literal is a string literal ending in ".htm" or ".html", the
// suffixes that make a script's answer a page file. It is looked up under
// pageDir, or under Root when it is a full "data/html/script/..." path. A
// full path elsewhere under data/html is outside the index and not checked.
// A literal starting with '-', '.' or '/', or holding a '%', is a fragment
// of a name built at run time and is not checked.
//
// exempt lists literals that are not checked: fragments the rules above
// cannot tell from names, and pages the script names but the datapack does
// not ship. An exemption that names an indexed page, or that the package
// never uses, is reported too.
//
// Test files are not read. Problems are returned sorted.
func CheckPackage(idx Index, dir, pageDir string, exempt ...string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("scriptpages: read package %s: %w", dir, err)
	}
	unused := make(map[string]bool, len(exempt))
	for _, lit := range exempt {
		unused[lit] = true
	}

	var problems []string
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("scriptpages: parse %s: %w", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if _, ok := unused[value]; ok {
				unused[value] = false
				return true
			}
			key, checked := pageKey(pageDir, value)
			if !checked {
				return true
			}
			if _, ok := idx[key]; !ok {
				pos := fset.Position(lit.Pos())
				problems = append(problems, fmt.Sprintf("%s:%d: page %q is not in the script page index (looked up %s)",
					filepath.Base(pos.Filename), pos.Line, value, key))
			}
			return true
		})
	}

	for _, lit := range exempt {
		if key, checked := pageKey(pageDir, lit); checked {
			if _, ok := idx[key]; ok {
				problems = append(problems, fmt.Sprintf("exemption %q names indexed page %s", lit, key))
				continue
			}
		}
		if unused[lit] {
			problems = append(problems, fmt.Sprintf("exemption %q is not used by the package", lit))
		}
	}
	slices.Sort(problems)
	return problems, nil
}

// pageKey returns the index name a page literal resolves to, and whether
// the literal is checked at all.
func pageKey(pageDir, lit string) (string, bool) {
	if !strings.HasSuffix(lit, ".htm") && !strings.HasSuffix(lit, ".html") {
		return "", false
	}
	full := strings.TrimPrefix(lit, "./")
	if rest, ok := strings.CutPrefix(full, Root+"/"); ok {
		return rest, true
	}
	if strings.HasPrefix(full, "data/html/") {
		return "", false
	}
	if strings.ContainsRune(lit, '%') || strings.ContainsAny(lit[:1], "-./") {
		return "", false
	}
	return pageDir + "/" + lit, true
}

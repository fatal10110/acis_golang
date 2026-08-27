// Package keyscan finds every config key literal read through
// config.Properties/config.Fields accessors in the Go source tree, so the
// supported-key registry in internal/config can be generated from actual
// readers instead of hand-maintained.
package keyscan

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const configImportPath = `"github.com/fatal10110/acis_golang/internal/config"`

var accessorMethods = map[string]bool{
	"String": true, "Bool": true, "Int": true, "Int64": true, "Float64": true,
	"Strings": true, "Bools": true, "Ints": true, "Int64s": true, "Float64s": true,
	"IntPairs": true, "IntPairsComma": true, "Lookup": true,
}

var loaderFuncs = map[string]bool{
	"LoadFile": true, "Parse": true, "ParseString": true, "NewFields": true,
}

// Scan walks the .go files under root (excluding tests) and returns every
// literal key passed to a config.Properties/config.Fields accessor method,
// sorted and de-duplicated.
func Scan(root string) ([]string, error) {
	keys := make(map[string]bool)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == "keyscan" || strings.HasPrefix(base, ".") || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		return scanFile(path, keys)
	})
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

func scanFile(path string, keys map[string]bool) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !strings.Contains(string(src), configImportPath) {
		return nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return err
	}

	configAlias := "config"
	for _, imp := range file.Imports {
		if imp.Path.Value == configImportPath && imp.Name != nil {
			configAlias = imp.Name.Name
		}
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		known := map[string]bool{}
		if fn.Recv != nil {
			addConfigParams(fn.Recv.List, configAlias, known)
		}
		if fn.Type.Params != nil {
			addConfigParams(fn.Type.Params.List, configAlias, known)
		}
		strLits := map[string][]string{}
		scanBlock(fn.Body, configAlias, known, strLits, keys)
	}
	return nil
}

func addConfigParams(fields []*ast.Field, configAlias string, known map[string]bool) {
	for _, f := range fields {
		if !isConfigPtrType(f.Type, configAlias) {
			continue
		}
		for _, name := range f.Names {
			known[name.Name] = true
		}
	}
}

func isConfigPtrType(expr ast.Expr, configAlias string) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != configAlias {
		return false
	}
	return sel.Sel.Name == "Properties" || sel.Sel.Name == "Fields"
}

// scanBlock walks statements in order, growing known as it finds
// assignments whose right-hand side is a config loader call or an
// already-known identifier, growing strLits as it finds a variable assigned
// a string literal key alias (e.g. `key := "RateDropAdena"`), and records
// every accessor call's literal or literal-aliased key.
func scanBlock(node ast.Node, configAlias string, known map[string]bool, strLits map[string][]string, keys map[string]bool) {
	ast.Inspect(node, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			for i, rhs := range stmt.Rhs {
				if i >= len(stmt.Lhs) {
					continue
				}
				id, ok := stmt.Lhs[i].(*ast.Ident)
				if !ok || id.Name == "_" {
					continue
				}
				if isConfigish(rhs, configAlias, known) {
					known[id.Name] = true
				}
				if lit, ok := rhs.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if v, err := strconv.Unquote(lit.Value); err == nil {
						strLits[id.Name] = append(strLits[id.Name], v)
					}
				}
			}
		case *ast.ValueSpec:
			for i, rhs := range stmt.Values {
				if i >= len(stmt.Names) {
					continue
				}
				if isConfigish(rhs, configAlias, known) {
					known[stmt.Names[i].Name] = true
				}
				if lit, ok := rhs.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if v, err := strconv.Unquote(lit.Value); err == nil {
						strLits[stmt.Names[i].Name] = append(strLits[stmt.Names[i].Name], v)
					}
				}
			}
		case *ast.CallExpr:
			recordAccessorCall(stmt, configAlias, known, strLits, keys)
		}
		return true
	})
}

func isConfigish(expr ast.Expr, configAlias string, known map[string]bool) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return known[e.Name]
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == configAlias && loaderFuncs[sel.Sel.Name] {
				return true
			}
		}
	}
	return false
}

func recordAccessorCall(call *ast.CallExpr, configAlias string, known map[string]bool, strLits map[string][]string, keys map[string]bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !accessorMethods[sel.Sel.Name] || len(call.Args) == 0 {
		return
	}
	if !receiverIsConfigish(sel.X, configAlias, known) {
		return
	}
	switch arg := call.Args[0].(type) {
	case *ast.BasicLit:
		if arg.Kind != token.STRING {
			return
		}
		if key, err := strconv.Unquote(arg.Value); err == nil {
			keys[key] = true
		}
	case *ast.Ident:
		for _, key := range strLits[arg.Name] {
			keys[key] = true
		}
	}
}

func receiverIsConfigish(expr ast.Expr, configAlias string, known map[string]bool) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return known[e.Name]
	case *ast.CallExpr:
		return isConfigish(e, configAlias, known)
	}
	return false
}

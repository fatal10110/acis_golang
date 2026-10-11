package script

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The section 6 rule of docs/script-engine-plan.md: state a script shares
// between the players and NPCs it serves lives in the engine's cells only.
// A script package (every package below this one) may not declare a
// package-level variable, start a goroutine, import sync or sync/atomic,
// or let a closure inside a constructor capture a local that is mutable: a
// constructor is a function returning a Script, Hooks, Maker or MakerHooks
// of this package, and a local is mutable when anything but its own
// declaration writes it (an assignment, an increment, its address taken,
// a delete, clear or copy into it, or a send on it).

// Every committed script package keeps to the rule.
func TestScriptPackagesKeepSharedStateInCells(t *testing.T) {
	dirs := scriptPackageDirs(t, ".")
	for _, known := range []string{filepath.Join("quest", "q001"), "maker", filepath.Join("ai", "group")} {
		if !slices.Contains(dirs, known) {
			t.Fatalf("script packages %v miss %s", dirs, known)
		}
	}
	for _, dir := range dirs {
		if v := checkScriptState(t, dir); len(v) > 0 {
			t.Errorf("%s:\n  %s", dir, strings.Join(v, "\n  "))
		}
	}
}

// The check reports each kind of breach in the fixture, and nothing for
// the allowed shapes beside them.
func TestScriptStateCheckCatchesFixture(t *testing.T) {
	got := checkScriptState(t, filepath.Join("testdata", "statecheck", "bad"))
	want := []string{
		"bad.go:5: imports sync",
		"bad.go:8: package-level variable counter",
		"bad.go:10: package-level variable _",
		"bad.go:22: closure captures mutable constructor-local count",
		"bad.go:26: closure captures mutable constructor-local seen",
		"bad.go:28: closure captures mutable constructor-local other",
		"bad.go:29: go statement",
		"bad.go:34: closure captures mutable constructor-local ptr",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("violations:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	if got := checkScriptState(t, filepath.Join("testdata", "statecheck", "good")); len(got) > 0 {
		t.Fatalf("allowed shapes reported:\n  %s", strings.Join(got, "\n  "))
	}
}

// scriptPackageDirs returns every directory below root holding Go files,
// testdata excluded.
func scriptPackageDirs(t *testing.T, root string) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if dir := filepath.Dir(p); dir != root && strings.HasSuffix(p, ".go") && !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return dirs
}

// stubImporter hands every import back as an empty package: the check
// resolves only the package's own identifiers, so the type errors that
// leaves are ignored.
type stubImporter struct{}

func (stubImporter) Import(p string) (*types.Package, error) {
	name := path.Base(p)
	if i := strings.IndexAny(name, ".-"); i > 0 {
		name = name[:i]
	}
	pkg := types.NewPackage(p, name)
	pkg.MarkComplete()
	return pkg, nil
}

// checkScriptState returns the rule breaches of the non-test Go files of
// dir, each as "file:line: what", in file and line order.
func checkScriptState(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}
	conf := types.Config{Importer: stubImporter{}, Error: func(error) {}}
	_, _ = conf.Check(dir, fset, files, info)

	var out []string
	report := func(pos token.Pos, format string, args ...any) {
		p := fset.Position(pos)
		out = append(out, fmt.Sprintf("%s:%d: %s", filepath.Base(p.Filename), p.Line, fmt.Sprintf(format, args...)))
	}
	for _, f := range files {
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == "sync" || p == "sync/atomic" {
				report(imp.Pos(), "imports %s", p)
			}
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, spec := range d.Specs {
					for _, name := range spec.(*ast.ValueSpec).Names {
						report(name.Pos(), "package-level variable %s", name.Name)
					}
				}
			case *ast.FuncDecl:
				if d.Body == nil {
					continue
				}
				ast.Inspect(d.Body, func(n ast.Node) bool {
					if g, ok := n.(*ast.GoStmt); ok {
						report(g.Pos(), "go statement")
					}
					return true
				})
				if isConstructor(d) {
					checkConstructorCaptures(d, info, report)
				}
			}
		}
	}
	slices.SortStableFunc(out, func(a, b string) int {
		fa, la := splitPos(a)
		fb, lb := splitPos(b)
		if c := strings.Compare(fa, fb); c != 0 {
			return c
		}
		return la - lb
	})
	return out
}

func splitPos(s string) (string, int) {
	parts := strings.SplitN(s, ":", 3)
	line, _ := strconv.Atoi(parts[1])
	return parts[0], line
}

// isConstructor reports whether d returns a Script, Hooks, Maker or
// MakerHooks of the script package.
func isConstructor(d *ast.FuncDecl) bool {
	if d.Type.Results == nil {
		return false
	}
	for _, r := range d.Type.Results.List {
		if sel, ok := r.Type.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "script" {
				switch sel.Sel.Name {
				case "Script", "Hooks", "Maker", "MakerHooks":
					return true
				}
			}
		}
	}
	return false
}

// checkConstructorCaptures reports each local of the constructor d, not
// declared inside a closure, that a closure in d uses while something
// other than its declaration writes it.
func checkConstructorCaptures(d *ast.FuncDecl, info *types.Info, report func(token.Pos, string, ...any)) {
	locals := map[types.Object]bool{}
	collect := func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		if id, ok := n.(*ast.Ident); ok {
			if v, ok := info.Defs[id].(*types.Var); ok && !v.IsField() {
				locals[v] = true
			}
		}
		return true
	}
	ast.Inspect(d.Type, collect)
	ast.Inspect(d.Body, collect)

	written := map[types.Object]bool{}
	markRoot := func(e ast.Expr) {
		for {
			switch x := e.(type) {
			case *ast.ParenExpr:
				e = x.X
			case *ast.IndexExpr:
				e = x.X
			case *ast.SelectorExpr:
				e = x.X
			case *ast.StarExpr:
				e = x.X
			case *ast.Ident:
				if obj := info.Uses[x]; obj != nil {
					written[obj] = true
				}
				return
			default:
				return
			}
		}
	}
	ast.Inspect(d.Body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range s.Lhs {
				markRoot(lhs)
			}
		case *ast.IncDecStmt:
			markRoot(s.X)
		case *ast.RangeStmt:
			if s.Tok == token.ASSIGN {
				if s.Key != nil {
					markRoot(s.Key)
				}
				if s.Value != nil {
					markRoot(s.Value)
				}
			}
		case *ast.UnaryExpr:
			if s.Op == token.AND {
				markRoot(s.X)
			}
		case *ast.SendStmt:
			markRoot(s.Chan)
		case *ast.CallExpr:
			if id, ok := s.Fun.(*ast.Ident); ok && len(s.Args) > 0 {
				if _, builtin := info.Uses[id].(*types.Builtin); builtin {
					switch id.Name {
					case "delete", "clear", "copy", "close":
						markRoot(s.Args[0])
					}
				}
			}
		}
		return true
	})

	reported := map[types.Object]bool{}
	ast.Inspect(d.Body, func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		ast.Inspect(lit.Body, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			obj := info.Uses[id]
			if obj != nil && locals[obj] && written[obj] && !reported[obj] {
				reported[obj] = true
				report(id.Pos(), "closure captures mutable constructor-local %s", obj.Name())
			}
			return true
		})
		return false
	})
}

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
// write through a function's receiver or parameters (a hook bound as a
// method value reaches its receiver's state on every invocation), or let a
// closure inside a constructor capture a local that is mutable: a
// constructor is a function whose result type mentions a Script, Hooks,
// Maker, MakerHooks, Catalog or MakerCatalog of this package (a pointer to
// one or a factory of one included), and a local is mutable when anything
// but its own declaration writes it (an assignment, an increment, its
// address taken, a delete, clear or copy into it, or a send on it).
//
// State a family of scripts shares (the third-class saga core and its 31
// sagas) follows from the same rule: a value of a family type the catalog
// builds once and hands to the constructor of every member, holding engine
// cells only. Its plain fields can only be read: writing one through the
// constructor's parameter, a hook's capture of it or a method's receiver is
// each a breach above.

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
		"family.go:13: bump writes through its receiver f",
		"family.go:18: Member writes through its parameter f",
		"family.go:20: closure captures mutable constructor-local f",
		"receiver.go:21: onTalk writes through its receiver q",
		"receiver.go:30: onFirstTalk writes through its receiver q",
		"receiver.go:36: remember writes through its parameter q",
		"receiver.go:44: closure captures mutable constructor-local count",
		"receiver.go:54: closure captures mutable constructor-local count",
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
				checkReceiverWrites(d, info, report)
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

// isConstructor reports whether a result of d mentions a Script, Hooks,
// Maker, MakerHooks, Catalog or MakerCatalog of the script package anywhere
// in its type: a pointer to one, a factory returning one, or a collection
// of them builds hooks as much as the plain value does.
func isConstructor(d *ast.FuncDecl) bool {
	if d.Type.Results == nil {
		return false
	}
	found := false
	for _, r := range d.Type.Results.List {
		ast.Inspect(r.Type, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return !found
			}
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "script" {
				switch sel.Sel.Name {
				case "Script", "Hooks", "Maker", "MakerHooks", "Catalog", "MakerCatalog":
					found = true
				}
			}
			return false
		})
	}
	return found
}

// forEachWrite calls write with the root identifier of every place n
// writes: an assignment or increment target, a range variable assigned
// with =, an operand whose address is taken, the first argument of
// delete, clear, copy or close, and a channel sent on. through says
// whether the write reaches the root through a field, index or pointer
// rather than replacing the variable itself.
func forEachWrite(n ast.Node, info *types.Info, write func(root *ast.Ident, through bool)) {
	mark := func(e ast.Expr) {
		through := false
		for {
			switch x := e.(type) {
			case *ast.ParenExpr:
				e = x.X
			case *ast.IndexExpr:
				e, through = x.X, true
			case *ast.IndexListExpr:
				e, through = x.X, true
			case *ast.SelectorExpr:
				e, through = x.X, true
			case *ast.StarExpr:
				e, through = x.X, true
			case *ast.Ident:
				write(x, through)
				return
			default:
				return
			}
		}
	}
	ast.Inspect(n, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range s.Lhs {
				mark(lhs)
			}
		case *ast.IncDecStmt:
			mark(s.X)
		case *ast.RangeStmt:
			if s.Tok == token.ASSIGN {
				if s.Key != nil {
					mark(s.Key)
				}
				if s.Value != nil {
					mark(s.Value)
				}
			}
		case *ast.UnaryExpr:
			if s.Op == token.AND {
				mark(s.X)
			}
		case *ast.SendStmt:
			mark(s.Chan)
		case *ast.CallExpr:
			if id, ok := s.Fun.(*ast.Ident); ok && len(s.Args) > 0 {
				if _, builtin := info.Uses[id].(*types.Builtin); builtin {
					switch id.Name {
					case "delete", "clear", "copy", "close":
						// Every one of them writes what its operand
						// refers to, never the variable itself.
						mark(&ast.StarExpr{X: s.Args[0]})
					}
				}
			}
		}
		return true
	})
}

// checkReceiverWrites reports each write of the function d through its
// receiver or one of its parameters. A hook bound as a method value, or a
// helper handed a pointer, reaches state every invocation shares through
// them exactly as a closure does through a captured local.
func checkReceiverWrites(d *ast.FuncDecl, info *types.Info, report func(token.Pos, string, ...any)) {
	params := map[types.Object]string{}
	add := func(fields *ast.FieldList, kind string) {
		if fields == nil {
			return
		}
		for _, f := range fields.List {
			for _, name := range f.Names {
				if obj := info.Defs[name]; obj != nil {
					params[obj] = kind
				}
			}
		}
	}
	add(d.Recv, "receiver")
	add(d.Type.Params, "parameter")
	reported := map[types.Object]bool{}
	forEachWrite(d.Body, info, func(root *ast.Ident, through bool) {
		obj := info.Uses[root]
		kind, ok := params[obj]
		if !ok || !through || reported[obj] {
			return
		}
		reported[obj] = true
		report(root.Pos(), "%s writes through its %s %s", d.Name.Name, kind, obj.Name())
	})
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
	forEachWrite(d.Body, info, func(root *ast.Ident, _ bool) {
		if obj := info.Uses[root]; obj != nil {
			written[obj] = true
		}
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

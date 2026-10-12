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
// address taken, a delete, clear, copy or append into it, or a send on it).
// A write through a local is also a write through every variable that
// local, or a field of it, was assigned from: s := f.seen; s[k] = true
// writes f, as does w.m = f.seen; w.m[k] = true, and p := ptr; *p = 1
// writes ptr.
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
		"alias.go:10: closure captures mutable constructor-local f",
		"alias.go:11: Alias writes through its parameter f",
		"alias.go:20: closure captures mutable constructor-local seen",
		"alias.go:30: closure captures mutable constructor-local f",
		"alias.go:32: Chain writes through its parameter f",
		"alias.go:41: closure captures mutable constructor-local total",
		"alias.go:52: closure captures mutable constructor-local order",
		"alias.go:68: closure captures mutable constructor-local r",
		"alias.go:69: Ranged writes through its parameter r",
		"alias.go:78: mark writes through its receiver r",
		"alias.go:85: keep writes through its parameter r",
		"alias.go:91: closure captures mutable constructor-local f",
		"alias.go:92: Wrapped writes through its parameter f",
		"alias.go:110: ResliceAppend writes through its parameter b",
		"alias.go:110: closure captures mutable constructor-local b",
		"alias.go:119: ResliceCopy writes through its parameter b",
		"alias.go:119: closure captures mutable constructor-local b",
		"alias.go:127: ResliceStore writes through its parameter b",
		"alias.go:127: closure captures mutable constructor-local b",
		"alias.go:136: closure captures mutable constructor-local f",
		"alias.go:137: FieldAssign writes through its parameter f",
		"alias.go:145: closure captures mutable constructor-local b",
		"alias.go:147: Switched writes through its parameter b",
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
	info := &types.Info{
		Defs:      map[*ast.Ident]types.Object{},
		Uses:      map[*ast.Ident]types.Object{},
		Types:     map[ast.Expr]types.TypeAndValue{},
		Implicits: map[ast.Node]types.Object{},
	}
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

// forEachWrite calls write with the variable behind every place n writes:
// an assignment or increment target, a range variable assigned with =, an
// operand whose address is taken, the first argument of delete, clear,
// copy, close or append, and a channel sent on. through says whether the
// write reaches the variable through a field, index, slice or pointer
// rather than replacing the variable itself. A write of a local of n that
// goes through a reference it holds (a pointer, map, slice or interface)
// is also a write through every variable that local was ever assigned from
// (see aliasSources), at the same position.
func forEachWrite(n ast.Node, info *types.Info, write func(v types.Object, pos token.Pos, through bool)) {
	aliases := aliasSources(n, info)
	mark := func(e ast.Expr) {
		id, through, deref := placeRoot(e, info)
		if id == nil {
			return
		}
		v := info.Uses[id]
		if v == nil {
			return
		}
		write(v, id.Pos(), through)
		if deref {
			for _, src := range sharedWith(v, aliases) {
				write(src, id.Pos(), true)
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
					case "delete", "clear", "copy", "close", "append":
						// Every one of them writes what its operand
						// refers to (append the backing array past the
						// length), never the variable itself.
						mark(&ast.StarExpr{X: s.Args[0]})
					}
				}
			}
		}
		return true
	})
}

// placeRoot walks a written place down to the variable it belongs to, if
// any. through says whether the place is a part of the variable (a field,
// element, slice or pointee) rather than the variable itself, deref whether
// reaching it follows a reference the variable holds instead of staying in
// the variable's own storage (a field of a struct value, an element of an
// array value). A step through a value of unresolved type counts as a
// reference.
func placeRoot(e ast.Expr, info *types.Info) (id *ast.Ident, through, deref bool) {
	viaRef := func(x ast.Expr) bool {
		tv, ok := info.Types[x]
		if !ok || tv.Type == nil {
			return true
		}
		switch tv.Type.Underlying().(type) {
		case *types.Struct, *types.Array:
			return false
		}
		return true
	}
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.IndexExpr:
			deref = deref || viaRef(x.X)
			e, through = x.X, true
		case *ast.IndexListExpr:
			deref = deref || viaRef(x.X)
			e, through = x.X, true
		case *ast.SliceExpr:
			deref = deref || viaRef(x.X)
			e, through = x.X, true
		case *ast.SelectorExpr:
			deref = deref || viaRef(x.X)
			e, through = x.X, true
		case *ast.StarExpr:
			e, through, deref = x.X, true, true
		case *ast.Ident:
			return x, through, deref
		default:
			return nil, false, false
		}
	}
}

// aliasSources maps each variable assigned anywhere in n to the variables
// its value may share memory with: the root of every value assigned to it,
// or stored in its own storage (a field of it, an element of it as an
// array), by =, :=, var, range or a type switch guard. A value stored past
// a reference the variable holds lands in memory the variable only refers
// to, and that store is itself a write through the variable. A variable
// whose type holds no reference holds a copy and aliases nothing. The map is flow- and field-insensitive, so a local
// that once held a captured map keeps aliasing it after it is reassigned;
// the check errs toward reporting.
func aliasSources(n ast.Node, info *types.Info) map[types.Object][]types.Object {
	aliases := map[types.Object][]types.Object{}
	alias := func(v types.Object, rhs ast.Expr) {
		if v == nil || !mayShare(v.Type()) {
			return
		}
		for _, src := range valueRoots(rhs, info) {
			if src != v {
				aliases[v] = append(aliases[v], src)
			}
		}
	}
	bind := func(lhs, rhs ast.Expr) {
		if id, _, deref := placeRoot(lhs, info); id != nil && !deref {
			alias(info.ObjectOf(id), rhs)
		}
	}
	bindAll := func(lhs, rhs []ast.Expr) {
		switch {
		case len(lhs) == len(rhs):
			for i := range lhs {
				bind(lhs[i], rhs[i])
			}
		case len(rhs) == 1:
			// v, ok := m[k] and its kin: the first result is the value.
			bind(lhs[0], rhs[0])
		}
	}
	ast.Inspect(n, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			if s.Tok == token.ASSIGN || s.Tok == token.DEFINE {
				bindAll(s.Lhs, s.Rhs)
			}
		case *ast.ValueSpec:
			names := make([]ast.Expr, len(s.Names))
			for i, name := range s.Names {
				names[i] = name
			}
			bindAll(names, s.Values)
		case *ast.RangeStmt:
			for _, v := range []ast.Expr{s.Key, s.Value} {
				if v != nil {
					bind(v, s.X)
				}
			}
		case *ast.TypeSwitchStmt:
			// The guard's variable is a separate object in every clause,
			// each holding the asserted value.
			guard, ok := s.Assign.(*ast.AssignStmt)
			if !ok || len(guard.Rhs) != 1 {
				break
			}
			for _, c := range s.Body.List {
				alias(info.Implicits[c], guard.Rhs[0])
			}
		}
		return true
	})
	return aliases
}

// valueRoots returns the variables a value is read out of: the root of a
// field, index, slice, dereference, address or type assertion chain, and
// the roots of every element of a composite literal or argument of append.
// A value whose type holds no reference (a number, a string, a struct or
// array of them) is a copy and has none, as has any other value, a call's
// result among them. A slice spread into append is copied element by
// element, so it shares only when its element type does.
func valueRoots(e ast.Expr, info *types.Info) []types.Object {
	if tv, ok := info.Types[e]; ok && !mayShare(tv.Type) {
		return nil
	}
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.SelectorExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		case *ast.SliceExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.TypeAssertExpr:
			e = x.X
		case *ast.KeyValueExpr:
			return valueRoots(x.Value, info)
		case *ast.UnaryExpr:
			if x.Op != token.AND {
				return nil
			}
			e = x.X
		case *ast.CompositeLit:
			return rootsOf(x.Elts, info)
		case *ast.CallExpr:
			id, ok := x.Fun.(*ast.Ident)
			if !ok || id.Name != "append" {
				return nil
			}
			if _, builtin := info.Uses[id].(*types.Builtin); !builtin {
				return nil
			}
			args := x.Args
			if last := len(args) - 1; x.Ellipsis.IsValid() && last > 0 {
				if tv, ok := info.Types[args[last]]; ok {
					if sl, ok := tv.Type.Underlying().(*types.Slice); ok && !mayShare(sl.Elem()) {
						args = args[:last]
					}
				}
			}
			return rootsOf(args, info)
		case *ast.Ident:
			if v := info.Uses[x]; v != nil {
				return []types.Object{v}
			}
			return nil
		default:
			return nil
		}
	}
}

func rootsOf(es []ast.Expr, info *types.Info) []types.Object {
	var out []types.Object
	for _, e := range es {
		out = append(out, valueRoots(e, info)...)
	}
	return out
}

// mayShare reports whether a value of type t can reach memory a copy of
// it shares: whether t is or holds a pointer, slice, map, channel, function
// or interface, or is a type the stub importer left unresolved.
func mayShare(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Kind() == types.Invalid || u.Kind() == types.UnsafePointer
	case *types.Array:
		return mayShare(u.Elem())
	case *types.Struct:
		for f := range u.Fields() {
			if mayShare(f.Type()) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// sharedWith returns every variable v aliases, directly or through other
// aliases, v itself excluded.
func sharedWith(v types.Object, aliases map[types.Object][]types.Object) []types.Object {
	var out []types.Object
	seen := map[types.Object]bool{v: true}
	stack := []types.Object{v}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, src := range aliases[cur] {
			if !seen[src] {
				seen[src] = true
				out = append(out, src)
				stack = append(stack, src)
			}
		}
	}
	return out
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
	forEachWrite(d.Body, info, func(v types.Object, pos token.Pos, through bool) {
		kind, ok := params[v]
		if !ok || !through || reported[v] {
			return
		}
		reported[v] = true
		report(pos, "%s writes through its %s %s", d.Name.Name, kind, v.Name())
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
	forEachWrite(d.Body, info, func(v types.Object, _ token.Pos, _ bool) {
		written[v] = true
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

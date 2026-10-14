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
// address taken, a delete, clear, copy or append into it, a send on it, or
// a call it is handed to that may write it: slices.Sort(order), a function
// value, an engine call). A write through a local is also a write through
// every variable that local, or a field of it, was assigned from, or that
// was stored behind a reference the write goes past: s := f.seen;
// s[k] = true writes f, as do w.m = f.seen; w.m[k] = true and
// m[0] = f.seen; m[0][k] = true, and p := ptr; *p = 1 writes ptr. A call's
// result counts as read out of what the call may return: f.seenMap()[k] =
// true writes f when seenMap returns f's map.
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
		"calls.go:24: Sorted writes through its parameter o",
		"calls.go:24: closure captures mutable constructor-local o",
		"calls.go:33: Copied writes through its parameter o",
		"calls.go:33: closure captures mutable constructor-local o",
		"calls.go:42: Resorted writes through its parameter o",
		"calls.go:42: closure captures mutable constructor-local o",
		"calls.go:50: closure captures mutable constructor-local o",
		"calls.go:51: MethodResult writes through its parameter o",
		"calls.go:59: DirectResult writes through its parameter o",
		"calls.go:59: closure captures mutable constructor-local o",
		"calls.go:68: Callback writes through its parameter o",
		"calls.go:68: closure captures mutable constructor-local o",
		"calls.go:77: Engine writes through its parameter o",
		"calls.go:77: closure captures mutable constructor-local o",
		"calls.go:86: closure captures mutable constructor-local o",
		"calls.go:87: Nested writes through its parameter o",
		"calls.go:94: sortAll writes through its parameter o",
		"calls.go:102: stash writes through its parameter o",
		"calls.go:110: closure captures mutable constructor-local o",
		"calls.go:112: Unpacked writes through its parameter o",
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
	info := &pkgInfo{
		Info: &types.Info{
			Defs:      map[*ast.Ident]types.Object{},
			Uses:      map[*ast.Ident]types.Object{},
			Types:     map[ast.Expr]types.TypeAndValue{},
			Implicits: map[ast.Node]types.Object{},
		},
		decls:   map[*types.Func]*ast.FuncDecl{},
		results: map[*types.Func][]bool{},
	}
	conf := types.Config{Importer: stubImporter{}, Error: func(error) {}}
	_, _ = conf.Check(dir, fset, files, info.Info)
	for _, f := range files {
		for _, decl := range f.Decls {
			if d, ok := decl.(*ast.FuncDecl); ok && d.Body != nil {
				if fn, ok := info.Defs[d.Name].(*types.Func); ok {
					info.decls[fn] = d
				}
			}
		}
	}

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

// pkgInfo is the type information of the checked package, with its
// function declarations and what their results may share.
type pkgInfo struct {
	*types.Info
	decls   map[*types.Func]*ast.FuncDecl
	results map[*types.Func][]bool // see resultShares
}

// resultShares reports, for the package's function fn, which of its
// receiver (first, false for a function) and parameters (in order) its
// results may share memory with: those a returned value is read out of,
// directly, through a local alias, or through a value stored behind a
// returned reference. A method returning its receiver's map shares its
// receiver; one building a new slice of its receiver's numbers shares
// nothing. A function whose declaration is not at hand, or that is still
// being summarized (recursion), shares everything.
func (info *pkgInfo) resultShares(fn *types.Func) []bool {
	if r, ok := info.results[fn]; ok && r != nil {
		return r
	}
	sig, _ := fn.Type().(*types.Signature)
	d := info.decls[fn]
	if sig == nil {
		return nil
	}
	all := make([]bool, 1+sig.Params().Len())
	for i := range all {
		all[i] = true
	}
	if _, inProgress := info.results[fn]; d == nil || inProgress {
		return all
	}
	info.results[fn] = nil
	a := aliasSources(d.Body, info)
	var roots []types.Object
	for v := range sig.Results().Variables() {
		roots = append(roots, v)
	}
	ast.Inspect(d.Body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			roots = append(roots, rootsOf(s.Results, info)...)
		}
		return true
	})
	reach := map[types.Object]bool{}
	for _, r := range roots {
		reach[r] = true
		for _, src := range a.reached(r, nil) {
			reach[src] = true
		}
		for _, st := range a.stored {
			if st.holder == r || slices.Contains(a.sharedWith(r), st.holder) {
				reach[st.src] = true
				for _, src := range a.sharedWith(st.src) {
					reach[src] = true
				}
			}
		}
	}
	out := make([]bool, len(all))
	if recv := sig.Recv(); recv != nil {
		out[0] = reach[recv]
	}
	for i, v := range slices.Collect(sig.Params().Variables()) {
		out[1+i] = reach[v]
	}
	info.results[fn] = out
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
// operand whose address is taken, a channel sent on, and every argument a
// call may write through (see writtenArgs). through says whether the write
// reaches the variable through a field, index, slice or pointer rather than
// replacing the variable itself. A write of a local of n that goes through
// a reference it holds (a pointer, map, slice or interface) is also a write
// through every variable that local was ever assigned from, or that was
// stored behind the reference written (see aliasSources), at the same
// position. A place reached through a call's result or a literal is written
// through every variable that value may share memory with (see valueRoots).
func forEachWrite(n ast.Node, info *pkgInfo, write func(v types.Object, pos token.Pos, through bool)) {
	aliases := aliasSources(n, info)
	mark := func(e ast.Expr) {
		root, steps := placePath(e, info)
		deref := followsRef(steps)
		id, ok := root.(*ast.Ident)
		if !ok {
			if deref {
				for _, v := range valueRoots(root, info) {
					write(v, root.Pos(), true)
					for _, src := range aliases.reached(v, nil) {
						write(src, root.Pos(), true)
					}
				}
			}
			return
		}
		v := info.Uses[id]
		if v == nil {
			return
		}
		write(v, id.Pos(), len(steps) > 0)
		if deref {
			for _, src := range aliases.reached(v, steps) {
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
			for _, arg := range writtenArgs(s, info) {
				// A call writes what its argument refers to, never the
				// caller's variable itself.
				mark(&ast.StarExpr{X: arg})
			}
		}
		return true
	})
}

// libraryCall is what a function of another package does with its
// arguments, as far as the check knows it.
type libraryCall int

const (
	// writesAll: the check does not know the function; it may write
	// through every argument.
	writesAll libraryCall = iota
	// readsAll: the function only reads its arguments.
	readsAll
	// writesFirst: the function writes through its first argument only.
	writesFirst
	// copiesAll: the function only reads its arguments and returns a new
	// container holding their elements.
	copiesAll
)

// libraryCalls lists the functions of other packages the check knows by
// "import/path.Name"; readOnlyPackages lists packages whose every function
// only reads its arguments.
var (
	libraryCalls = map[string]libraryCall{
		"errors.Is":               readsAll,
		"errors.New":              readsAll,
		"fmt.Errorf":              readsAll,
		"fmt.Sprint":              readsAll,
		"fmt.Sprintf":             readsAll,
		"fmt.Sprintln":            readsAll,
		"maps.All":                readsAll,
		"maps.Clone":              copiesAll,
		"maps.Collect":            copiesAll,
		"maps.Copy":               writesFirst,
		"maps.DeleteFunc":         writesFirst,
		"maps.Equal":              readsAll,
		"maps.EqualFunc":          readsAll,
		"maps.Insert":             writesFirst,
		"maps.Keys":               readsAll,
		"maps.Values":             readsAll,
		"slices.All":              readsAll,
		"slices.Backward":         readsAll,
		"slices.BinarySearch":     readsAll,
		"slices.Clip":             writesFirst,
		"slices.Clone":            copiesAll,
		"slices.Collect":          copiesAll,
		"slices.Compact":          writesFirst,
		"slices.CompactFunc":      writesFirst,
		"slices.Compare":          readsAll,
		"slices.Concat":           copiesAll,
		"slices.Contains":         readsAll,
		"slices.ContainsFunc":     readsAll,
		"slices.Delete":           writesFirst,
		"slices.DeleteFunc":       writesFirst,
		"slices.Equal":            readsAll,
		"slices.EqualFunc":        readsAll,
		"slices.Grow":             writesFirst,
		"slices.Index":            readsAll,
		"slices.IndexFunc":        readsAll,
		"slices.Insert":           writesFirst,
		"slices.IsSorted":         readsAll,
		"slices.IsSortedFunc":     readsAll,
		"slices.Max":              readsAll,
		"slices.MaxFunc":          readsAll,
		"slices.Min":              readsAll,
		"slices.MinFunc":          readsAll,
		"slices.Replace":          writesFirst,
		"slices.Reverse":          writesFirst,
		"slices.Sort":             writesFirst,
		"slices.SortFunc":         writesFirst,
		"slices.SortStableFunc":   writesFirst,
		"slices.Sorted":           copiesAll,
		"slices.SortedFunc":       copiesAll,
		"slices.SortedStableFunc": copiesAll,
		"slices.Values":           readsAll,
		"sort.Float64s":           writesFirst,
		"sort.Ints":               writesFirst,
		"sort.Slice":              writesFirst,
		"sort.SliceStable":        writesFirst,
		"sort.Strings":            writesFirst,
	}
	readOnlyPackages = []string{"math", "strconv", "strings", "unicode", "unicode/utf8"}
)

// libraryFunc returns the "import/path.Name" of a call's function when it
// is a function of an imported package.
func libraryFunc(call *ast.CallExpr, info *pkgInfo) (string, bool) {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	pkg, ok := info.Uses[x].(*types.PkgName)
	if !ok {
		return "", false
	}
	return pkg.Imported().Path() + "." + sel.Sel.Name, true
}

// libraryEffect returns what the library function name does with its
// arguments.
func libraryEffect(name string) libraryCall {
	if slices.Contains(readOnlyPackages, name[:strings.LastIndex(name, ".")]) {
		return readsAll
	}
	if effect, ok := libraryCalls[name]; ok {
		return effect
	}
	return writesAll
}

// writtenArgs returns the arguments a call may write through: the first
// argument of delete, clear, copy, close and append (each writes what its
// operand refers to, append the backing array past the length), and every
// argument holding a reference (see holdsRef) handed to a function the
// check cannot follow: a function of another package it does not know as
// read-only (see libraryCalls), a function value, a method of an engine
// type. A function or method of the package itself is checked on its own,
// writes through its parameters included, and a conversion calls nothing.
func writtenArgs(call *ast.CallExpr, info *pkgInfo) []ast.Expr {
	if len(call.Args) == 0 || info.Types[call.Fun].IsType() {
		return nil
	}
	args := call.Args
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		switch info.Uses[fun].(type) {
		case *types.Builtin:
			switch fun.Name {
			case "delete", "clear", "copy", "close", "append":
				return args[:1]
			}
			return nil
		case *types.Func:
			return nil
		}
	case *ast.SelectorExpr:
		if name, ok := libraryFunc(call, info); ok {
			switch libraryEffect(name) {
			case readsAll, copiesAll:
				return nil
			case writesFirst:
				args = args[:1]
			}
		} else if _, ok := info.Uses[fun.Sel].(*types.Func); ok {
			return nil
		}
	}
	var out []ast.Expr
	for _, arg := range args {
		if tv, ok := info.Types[arg]; ok && tv.Type != nil && holdsRef(tv.Type) {
			out = append(out, arg)
		}
	}
	return out
}

// holdsRef reports whether a value of the resolved type t holds a pointer,
// slice, map, channel or interface a callee could write through. A value of
// a type the stub importer left unresolved is an engine value (a creature,
// an item, a cell): handing it to the engine is how a script acts, so it
// is not judged.
func holdsRef(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Kind() == types.UnsafePointer
	case *types.Signature:
		return false
	case *types.Array:
		return holdsRef(u.Elem())
	case *types.Struct:
		for f := range u.Fields() {
			if holdsRef(f.Type()) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// pathStep is one step from a variable down to a place in or behind it.
type pathStep struct {
	key string // ".name" for a field, "[]" for an element or slice, "*" for a pointee
	ref bool   // the step follows a reference rather than staying in the storage it starts from
}

// placePath walks a written place down to the expression it starts from,
// a variable or another value, and returns the steps from that expression
// to the place, outermost last. A step through a value of unresolved type
// counts as following a reference.
func placePath(e ast.Expr, info *pkgInfo) (ast.Expr, []pathStep) {
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
	var steps []pathStep
	for {
		var step pathStep
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
			continue
		case *ast.IndexExpr:
			step, e = pathStep{"[]", viaRef(x.X)}, x.X
		case *ast.IndexListExpr:
			step, e = pathStep{"[]", viaRef(x.X)}, x.X
		case *ast.SliceExpr:
			step, e = pathStep{"[]", viaRef(x.X)}, x.X
		case *ast.SelectorExpr:
			step, e = pathStep{"." + x.Sel.Name, viaRef(x.X)}, x.X
		case *ast.StarExpr:
			step, e = pathStep{"*", true}, x.X
		default:
			slices.Reverse(steps)
			return e, steps
		}
		steps = append(steps, step)
	}
}

// followsRef reports whether any of steps follows a reference.
func followsRef(steps []pathStep) bool {
	return slices.ContainsFunc(steps, func(s pathStep) bool { return s.ref })
}

// aliases is what aliasSources finds: the variables each variable's own
// value was taken from, and the values stored behind a reference a
// variable holds.
type aliases struct {
	direct map[types.Object][]types.Object
	stored []storedAlias
}

// storedAlias records that a value read out of src was stored at path
// behind a reference holder holds (m[k] = src.f, p.f = src).
type storedAlias struct {
	holder types.Object
	path   []pathStep
	src    types.Object
}

// aliasSources finds, for each variable assigned anywhere in n, the
// variables its value may share memory with: the root of every value
// assigned to it, or stored in its own storage (a field of it, an element
// of it as an array), by =, :=, var, range or a type switch guard. A value
// stored past a reference the variable holds lands in memory the variable
// only refers to; that store is itself a write through the variable, and
// is kept apart as a stored alias, which a later write reaches only when
// it goes past the stored value's place (see reached), or a variable reads
// the value back out of that place. A variable whose type holds no
// reference holds a copy and aliases nothing. Both are flow-insensitive, so a local that once held a captured map keeps aliasing it
// after it is reassigned; the check errs toward reporting.
func aliasSources(n ast.Node, info *pkgInfo) aliases {
	a := aliases{direct: map[types.Object][]types.Object{}}
	// read records the variables whose value was read out of a place below
	// another variable (x := m[0]), with that place's steps.
	type read struct {
		v, from types.Object
		path    []pathStep
	}
	var reads []read
	alias := func(v types.Object, rhs ast.Expr) {
		if v == nil || !mayShare(v.Type()) {
			return
		}
		for _, src := range valueRoots(rhs, info) {
			if src != v {
				a.direct[v] = append(a.direct[v], src)
			}
		}
		if root, steps := placePath(rhs, info); len(steps) > 0 {
			if id, ok := root.(*ast.Ident); ok && info.Uses[id] != nil {
				reads = append(reads, read{v: v, from: info.Uses[id], path: steps})
			}
		}
	}
	bind := func(lhs, rhs ast.Expr) {
		root, steps := placePath(lhs, info)
		id, ok := root.(*ast.Ident)
		if !ok {
			return
		}
		v := info.ObjectOf(id)
		if !followsRef(steps) {
			alias(v, rhs)
			return
		}
		for _, src := range valueRoots(rhs, info) {
			if v != nil && src != v {
				a.stored = append(a.stored, storedAlias{holder: v, path: steps, src: src})
			}
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
	// A value read out of the place a value was stored at, or from inside
	// it, is (part of) that stored value: m[0] = f.seen; x := m[0] makes x
	// an alias of f.
	for _, r := range reads {
		for _, st := range a.stored {
			if st.holder == r.from && len(st.path) <= len(r.path) && samePath(r.path[:len(st.path)], st.path) && st.src != r.v {
				a.direct[r.v] = append(a.direct[r.v], st.src)
			}
		}
	}
	return a
}

// valueRoots returns the variables a value is read out of: the root of a
// field, index, slice, dereference, address or type assertion chain, the
// roots of every element of a composite literal, and the roots of a call's
// result (see callRoots). A value whose type holds no reference (a number,
// a string, a struct or array of them) is a copy and has none, as has any
// other value.
func valueRoots(e ast.Expr, info *pkgInfo) []types.Object {
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
			return callRoots(x, info)
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

// callRoots returns the roots of a call's result. A conversion shares its
// operand. Of the builtins only append returns memory its operands share;
// a slice spread into it is copied element by element, so it shares only
// when its element type does. A standard library function that returns a
// new container (see libraryCalls) shares only arguments whose elements
// hold references, any other its every argument. A function or method of
// the package shares the receiver and arguments its declaration returns
// (see resultShares): the method seenMap returning its receiver's map is
// that map. The engine (a function of another package of the module, or a
// method of a type the stub importer left unresolved) returns engine
// values. Any other call, a function value's among them, may return its
// receiver or any argument.
func callRoots(call *ast.CallExpr, info *pkgInfo) []types.Object {
	if info.Types[call.Fun].IsType() {
		return rootsOf(call.Args, info)
	}
	var fn *types.Func
	var recv ast.Expr
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		if _, builtin := info.Uses[fun].(*types.Builtin); builtin {
			if fun.Name != "append" {
				return nil
			}
			args := call.Args
			if last := len(args) - 1; call.Ellipsis.IsValid() && last > 0 && !elemsShare(args[last], info) {
				args = args[:last]
			}
			return rootsOf(args, info)
		}
		fn, _ = info.Uses[fun].(*types.Func)
	case *ast.SelectorExpr:
		if name, ok := libraryFunc(call, info); ok {
			if !isStandard(name) {
				return nil
			}
			if libraryEffect(name) != copiesAll {
				return rootsOf(call.Args, info)
			}
			var out []types.Object
			for _, arg := range call.Args {
				if elemsShare(arg, info) {
					out = append(out, valueRoots(arg, info)...)
				}
			}
			return out
		}
		fn, _ = info.Uses[fun.Sel].(*types.Func)
		recv = fun.X
	}
	if fn != nil && info.decls[fn.Origin()] != nil {
		shares := info.resultShares(fn.Origin())
		var out []types.Object
		if shares[0] && recv != nil {
			out = append(out, valueRoots(recv, info)...)
		}
		for i, arg := range call.Args {
			if shares[1+min(i, len(shares)-2)] {
				out = append(out, valueRoots(arg, info)...)
			}
		}
		return out
	}
	if _, ok := info.Types[call]; !ok {
		// The type check records no type for a call it could not resolve.
		return nil
	}
	out := rootsOf(call.Args, info)
	if recv != nil {
		out = append(out, valueRoots(recv, info)...)
	}
	return out
}

// isStandard reports whether the library function name ("import/path.Name")
// belongs to the standard library: its path's first element has no dot.
func isStandard(name string) bool {
	first, _, _ := strings.Cut(name[:strings.LastIndex(name, ".")], "/")
	return !strings.Contains(first, ".")
}

// elemsShare reports whether the elements of the container e (a slice's
// or array's elements, a map's keys and values) may share memory with the
// container's. An iterator over a container, or another library call's
// result, of unresolved type has the elements of its first argument; any
// other unresolved value counts as sharing.
func elemsShare(e ast.Expr, info *pkgInfo) bool {
	if tv, ok := info.Types[e]; ok && tv.Type != nil {
		switch u := tv.Type.Underlying().(type) {
		case *types.Slice:
			return mayShare(u.Elem())
		case *types.Array:
			return mayShare(u.Elem())
		case *types.Map:
			return mayShare(u.Key()) || mayShare(u.Elem())
		case *types.Basic:
			return false
		default:
			return true
		}
	}
	if call, ok := ast.Unparen(e).(*ast.CallExpr); ok && len(call.Args) > 0 {
		if _, ok := libraryFunc(call, info); ok {
			return elemsShare(call.Args[0], info)
		}
	}
	return true
}

func rootsOf(es []ast.Expr, info *pkgInfo) []types.Object {
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
// direct aliases, v itself excluded.
func (a aliases) sharedWith(v types.Object) []types.Object {
	var out []types.Object
	seen := map[types.Object]bool{v: true}
	stack := []types.Object{v}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, src := range a.direct[cur] {
			if !seen[src] {
				seen[src] = true
				out = append(out, src)
				stack = append(stack, src)
			}
		}
	}
	return out
}

// reached returns every variable, v excluded, that a write through v at
// steps below it also writes through: each variable v shares memory with
// (see sharedWith), and each value stored behind a reference v or one of
// those holds when the write goes past the stored value's place and on
// through a reference. m[0] = f.seen; m[0][k] = true writes f, while
// q.ghost = e.NPC; q.talks++ does not write e. The steps of a stored
// value's place match field by field, every element step alike. A nil
// steps reaches the shared variables only.
func (a aliases) reached(v types.Object, steps []pathStep) []types.Object {
	group := append([]types.Object{v}, a.sharedWith(v)...)
	out := slices.Clone(group[1:])
	for _, st := range a.stored {
		if !past(steps, st.path) || !slices.ContainsFunc(append([]types.Object{st.holder}, a.sharedWith(st.holder)...), func(h types.Object) bool {
			return slices.Contains(group, h)
		}) {
			continue
		}
		for _, src := range append([]types.Object{st.src}, a.sharedWith(st.src)...) {
			if src != v && !slices.Contains(out, src) {
				out = append(out, src)
			}
		}
	}
	return out
}

// past reports whether a write at steps goes past the place at path and
// on through a reference: path is a strict prefix of steps, key by key,
// and a step after it follows a reference.
func past(steps, path []pathStep) bool {
	return len(steps) > len(path) && samePath(steps[:len(path)], path) && followsRef(steps[len(path):])
}

// samePath reports whether two paths take the same steps, field by field,
// every element step alike.
func samePath(a, b []pathStep) bool {
	return slices.EqualFunc(a, b, func(x, y pathStep) bool { return x.key == y.key })
}

// checkReceiverWrites reports each write of the function d through its
// receiver or one of its parameters. A hook bound as a method value, or a
// helper handed a pointer, reaches state every invocation shares through
// them exactly as a closure does through a captured local.
func checkReceiverWrites(d *ast.FuncDecl, info *pkgInfo, report func(token.Pos, string, ...any)) {
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
func checkConstructorCaptures(d *ast.FuncDecl, info *pkgInfo, report func(token.Pos, string, ...any)) {
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

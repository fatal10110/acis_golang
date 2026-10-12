// Package handleops derives, from the Go source of the script packages,
// which NPC handle operations each catalog script's hooks reach that exist
// for some NPC kinds only: the handle half of the script seam gate
// (docs/script-engine-plan.md, sections 4 and 8).
//
// An operation is kind-specific when it reaches the handle's hostile
// accessor, the one helper that panics on any NPC but a hostile one. A hook
// reaches an operation when its function, or anything it references in the
// script packages, references the operation. A call through the Hooks
// invokers (a parent call, a re-entry into the final script), through a
// Hooks field or through an engine function that runs the script's own
// hook reaches every function the script assigns to that hook, its
// ancestors' included. A call through any other function value that is not
// a local variable with seen assignments (a struct field, a map or slice
// element, a call result) reaches every function value of that signature
// the content packages make, and is an error when they make none; the engine's
// own function values count at the engine function that makes them. The
// result over-approximates and never misses a statically visible call.
package handleops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// ScriptPkg is the import path of the script engine package.
const ScriptPkg = "github.com/fatal10110/acis_golang/internal/gameserver/script"

// Program is the type-checked script engine package and every package
// below it.
type Program struct {
	fset *token.FileSet
	// pkgs are the checked packages by import path.
	pkgs map[string]*types.Package
	// decls are the function declarations of the checked packages, with the
	// type information of their package.
	decls map[*types.Func]decl
	// byName holds the checked methods by name, for the targets of an
	// interface method call.
	byName map[string][]*types.Func
	// kinded are the engine functions that reach the hostile accessor.
	kinded map[*types.Func]bool
	// hooks are the Hooks fields by object, valued with their name.
	hooks map[*types.Var]string
	// hooksType is the engine's Hooks type.
	hooksType *types.Named
	// funcVars holds, per variable of function type, the expressions
	// assigned to it.
	funcVars map[*types.Var][]node
	// dynVars are the variables of function type set from a value the scan
	// does not follow: a range variable, one side of a multi-value
	// assignment.
	dynVars map[*types.Var]bool
	// funcValues are the function values the content packages make: every
	// function literal and every function referenced other than called.
	funcValues []funcValue
	// reentry holds, per engine function, the Hooks fields of the script it
	// is called on that it runs.
	reentry map[*types.Func][]string
}

// funcValue is a function value made in the code at n, of type sig.
type funcValue struct {
	sig types.Type
	nd  node
}

type decl struct {
	fn   *ast.FuncDecl
	info *types.Info
}

type listed struct {
	ImportPath string
	Dir        string
	GoFiles    []string
	Export     string
}

// Load type-checks, from source, the packages matching patterns (the
// script engine and its content packages) inside the module at root; every
// other package is read from its compiled export data.
func Load(root string, patterns ...string) (*Program, error) {
	args := append([]string{"list", "-deps", "-export", "-json=ImportPath,Dir,GoFiles,Export"}, patterns...)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("handleops: go list: %w: %s", err, stderr.String())
	}
	var pkgs []listed
	for dec := json.NewDecoder(bytes.NewReader(out)); ; {
		var p listed
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("handleops: go list output: %w", err)
		}
		pkgs = append(pkgs, p)
	}
	exports := map[string]string{}
	for _, p := range pkgs {
		exports[p.ImportPath] = p.Export
	}
	prog := &Program{
		fset:     token.NewFileSet(),
		pkgs:     map[string]*types.Package{},
		decls:    map[*types.Func]decl{},
		byName:   map[string][]*types.Func{},
		hooks:    map[*types.Var]string{},
		funcVars: map[*types.Var][]node{},
		dynVars:  map[*types.Var]bool{},
	}
	compiled := importer.ForCompiler(prog.fset, "gc", func(path string) (io.ReadCloser, error) {
		f, ok := exports[path]
		if !ok || f == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(f)
	})
	imp := importerFunc(func(path string) (*types.Package, error) {
		if p, ok := prog.pkgs[path]; ok {
			return p, nil
		}
		if inScript(path) {
			return nil, fmt.Errorf("script package %s imported before it was checked", path)
		}
		return compiled.Import(path)
	})
	// go list -deps lists every package after its dependencies.
	for _, p := range pkgs {
		if !inScript(p.ImportPath) || len(p.GoFiles) == 0 {
			continue
		}
		if err := prog.check(p, imp); err != nil {
			return nil, err
		}
	}
	if err := prog.index(); err != nil {
		return nil, err
	}
	return prog, nil
}

type importerFunc func(string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

func inScript(path string) bool {
	return path == ScriptPkg || strings.HasPrefix(path, ScriptPkg+"/")
}

func (p *Program) check(l listed, imp types.Importer) error {
	var files []*ast.File
	for _, name := range l.GoFiles {
		f, err := parser.ParseFile(p.fset, filepath.Join(l.Dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("handleops: %w", err)
		}
		files = append(files, f)
	}
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	var errs []error
	conf := types.Config{Importer: imp, Error: func(err error) { errs = append(errs, err) }}
	pkg, _ := conf.Check(l.ImportPath, p.fset, files, info)
	if len(errs) > 0 {
		return fmt.Errorf("handleops: type-check %s: %w", l.ImportPath, errors.Join(errs...))
	}
	p.pkgs[l.ImportPath] = pkg
	for _, f := range files {
		p.indexFuncVars(f, info)
		if l.ImportPath != ScriptPkg {
			p.indexFuncValues(f, info)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			fn := info.Defs[fd.Name].(*types.Func)
			p.decls[fn] = decl{fd, info}
			if fd.Recv != nil {
				p.byName[fn.Name()] = append(p.byName[fn.Name()], fn)
			}
		}
	}
	return nil
}

// indexFuncVars records what f assigns to its variables of function type.
func (p *Program) indexFuncVars(f *ast.File, info *types.Info) {
	funcVar := func(e ast.Expr) *types.Var {
		if sel, ok := e.(*ast.SelectorExpr); ok {
			if info.Selections[sel] != nil {
				return nil
			}
			e = sel.Sel
		}
		id, ok := e.(*ast.Ident)
		if !ok {
			return nil
		}
		obj := info.Defs[id]
		if obj == nil {
			obj = info.Uses[id]
		}
		if v, ok := obj.(*types.Var); ok && isFuncVar(v) {
			return v
		}
		return nil
	}
	record := func(lhs []ast.Expr, rhs []ast.Expr) {
		for i, e := range lhs {
			v := funcVar(e)
			switch {
			case v == nil:
			case len(lhs) == len(rhs):
				p.funcVars[v] = append(p.funcVars[v], node{rhs[i], info})
			default:
				p.dynVars[v] = true
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			record(n.Lhs, n.Rhs)
		case *ast.ValueSpec:
			if len(n.Values) > 0 {
				lhs := make([]ast.Expr, len(n.Names))
				for i, id := range n.Names {
					lhs[i] = id
				}
				record(lhs, n.Values)
			}
		case *ast.RangeStmt:
			for _, e := range []ast.Expr{n.Key, n.Value} {
				if v := funcVar(e); v != nil {
					p.dynVars[v] = true
				}
			}
		}
		return true
	})
}

// indexFuncValues records the function values f makes: its function
// literals and the functions it references other than as the callee of a
// call.
func (p *Program) indexFuncValues(f *ast.File, info *types.Info) {
	called := map[ast.Expr]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			fun := ast.Unparen(call.Fun)
			switch ix := fun.(type) {
			case *ast.IndexExpr:
				fun = ast.Unparen(ix.X)
			case *ast.IndexListExpr:
				fun = ast.Unparen(ix.X)
			}
			called[fun] = true
		}
		return true
	})
	add := func(e ast.Expr) {
		if t := info.TypeOf(e); t != nil {
			if _, ok := t.Underlying().(*types.Signature); ok {
				p.funcValues = append(p.funcValues, funcValue{t, node{e, info}})
			}
		}
	}
	var inspect func(ast.Node) bool
	inspect = func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			add(n)
		case *ast.SelectorExpr:
			// The selector stands for the function, not its name.
			if _, ok := info.Uses[n.Sel].(*types.Func); ok && !called[n] {
				add(n)
			}
			ast.Inspect(n.X, inspect)
			return false
		case *ast.Ident:
			if _, ok := info.Uses[n].(*types.Func); ok && !called[n] {
				add(n)
			}
		}
		return true
	}
	ast.Inspect(f, inspect)
}

// isFuncVar reports whether v is a variable of function type that is no
// struct field.
func isFuncVar(v *types.Var) bool {
	_, ok := v.Type().Underlying().(*types.Signature)
	return ok && !v.IsField()
}

// index finds the Hooks fields and the engine functions that reach the
// hostile accessor.
func (p *Program) index() error {
	engine := p.pkgs[ScriptPkg]
	if engine == nil {
		return fmt.Errorf("handleops: %s not loaded", ScriptPkg)
	}
	hooks, _ := engine.Scope().Lookup("Hooks").(*types.TypeName)
	npcType, _ := engine.Scope().Lookup("NPC").(*types.TypeName)
	if hooks == nil || npcType == nil {
		return errors.New("handleops: the engine declares no Hooks or NPC type")
	}
	p.hooksType = hooks.Type().(*types.Named)
	st := p.hooksType.Underlying().(*types.Struct)
	for f := range st.Fields() {
		if strings.HasPrefix(f.Name(), "On") {
			p.hooks[f] = f.Name()
		}
	}
	seed, _, _ := types.LookupFieldOrMethod(types.NewPointer(npcType.Type()), true, engine, "hostile")
	accessor, ok := seed.(*types.Func)
	if !ok {
		return errors.New("handleops: NPC has no hostile accessor")
	}
	p.kinded = map[*types.Func]bool{accessor: true}
	for changed := true; changed; {
		changed = false
		for fn, d := range p.decls {
			if fn.Pkg() != engine || p.kinded[fn] {
				continue
			}
			for _, ref := range p.refs(d.fn.Body, d.info) {
				if p.kinded[ref] {
					p.kinded[fn] = true
					changed = true
					break
				}
			}
		}
	}
	p.indexReentry(engine)
	return nil
}

// indexReentry finds the engine functions that run a hook of the script
// they are called on: a method of Script that runs a hook of its receiver
// (directly or through an invoker), and every engine function that
// references one. A registry dispatch runs the hooks of the scripts bound
// to an event, each an invocation of its own that the gate checks against
// that script's bindings, so it is no re-entry.
func (p *Program) indexReentry(engine *types.Package) {
	script, _ := engine.Scope().Lookup("Script").(*types.TypeName)
	p.reentry = map[*types.Func][]string{}
	for fn, d := range p.decls {
		if script == nil || fn.Pkg() != engine || d.fn.Recv == nil || len(d.fn.Recv.List[0].Names) == 0 {
			continue
		}
		recv, _ := d.info.Defs[d.fn.Recv.List[0].Names[0]].(*types.Var)
		if recv == nil || !isNamed(recv.Type(), script) {
			continue
		}
		var hooks []string
		ast.Inspect(d.fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
			if !ok || rootObj(sel.X, d.info) != recv {
				return true
			}
			switch obj := d.info.Uses[sel.Sel].(type) {
			case *types.Func:
				if p.isInvoker(obj) {
					hooks = append(hooks, "On"+obj.Name())
				}
			case *types.Var:
				if name, ok := p.hooks[obj]; ok {
					hooks = append(hooks, name)
				}
			}
			return true
		})
		if len(hooks) > 0 {
			p.reentry[fn] = hooks
		}
	}
	for changed := true; changed; {
		changed = false
		for fn, d := range p.decls {
			if fn.Pkg() != engine {
				continue
			}
			for _, ref := range p.refs(d.fn.Body, d.info) {
				for _, h := range p.reentry[ref] {
					if !slices.Contains(p.reentry[fn], h) {
						p.reentry[fn] = append(p.reentry[fn], h)
						changed = true
					}
				}
			}
		}
	}
}

// isNamed reports whether t is the named type of obj or a pointer to it.
func isNamed(t types.Type, obj *types.TypeName) bool {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	return ok && named.Obj() == obj
}

// rootObj returns the object the selector chain e starts at, nil when it
// starts at no identifier.
func rootObj(e ast.Expr, info *types.Info) types.Object {
	for {
		switch x := ast.Unparen(e).(type) {
		case *ast.SelectorExpr:
			e = x.X
		case *ast.Ident:
			return info.Uses[x]
		default:
			return nil
		}
	}
}

// refs returns the functions n references, an interface method expanded to
// every checked method that may implement it.
func (p *Program) refs(n ast.Node, info *types.Info) []*types.Func {
	var out []*types.Func
	ast.Inspect(n, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		fn, ok := info.Uses[id].(*types.Func)
		if !ok {
			return true
		}
		out = append(out, p.targets(fn.Origin())...)
		return true
	})
	return out
}

// targets returns fn, or for an interface method the checked methods of
// that name whose receiver implements the interface.
func (p *Program) targets(fn *types.Func) []*types.Func {
	sig := fn.Type().(*types.Signature)
	if sig.Recv() == nil {
		return []*types.Func{fn}
	}
	iface, ok := sig.Recv().Type().Underlying().(*types.Interface)
	if !ok {
		return []*types.Func{fn}
	}
	var out []*types.Func
	for _, m := range p.byName[fn.Name()] {
		recv := m.Type().(*types.Signature).Recv().Type()
		if types.Implements(recv, iface) {
			out = append(out, m)
		}
	}
	return out
}

// Ops returns the name of every exported engine operation that exists for
// hostile NPCs only, sorted: "NPC.HP", "Spawner.DecayMe".
func (p *Program) Ops() []string {
	var out []string
	for fn := range p.kinded {
		if name, ok := opName(fn); ok {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// opName names fn as the gate does, false for an unexported one.
func opName(fn *types.Func) (string, bool) {
	if !fn.Exported() {
		return "", false
	}
	recv := fn.Type().(*types.Signature).Recv()
	if recv == nil {
		return fn.Name(), true
	}
	t := recv.Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok || !named.Obj().Exported() {
		return "", false
	}
	return named.Obj().Name() + "." + fn.Name(), true
}

// Catalog reads the script catalogs of the Go file at file: every
// composite literal of the engine's Catalog type, keyed by a constant
// scripts.xml path and valued with a constructor, a selector of a checked
// package's function or a function literal calling one.
func (p *Program) Catalog(file string) (map[string][]*types.Func, error) {
	f, err := parser.ParseFile(p.fset, file, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("handleops: %w", err)
	}
	imports := map[string]string{}
	for _, spec := range f.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		} else if pkg := p.pkgs[path]; pkg != nil {
			name = pkg.Name()
		}
		imports[name] = path
	}
	out := map[string][]*types.Func{}
	var bad []string
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isCatalogType(lit.Type, imports) {
			return true
		}
		for _, elt := range lit.Elts {
			kv := elt.(*ast.KeyValueExpr)
			key, ok := kv.Key.(*ast.BasicLit)
			if !ok || key.Kind != token.STRING {
				bad = append(bad, p.fset.Position(kv.Pos()).String()+": catalog key is not a string literal")
				continue
			}
			path, _ := strconv.Unquote(key.Value)
			var ctors []*types.Func
			ast.Inspect(kv.Value, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				x, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				if pkg := p.pkgs[imports[x.Name]]; pkg != nil && pkg.Path() != ScriptPkg {
					if fn, ok := pkg.Scope().Lookup(sel.Sel.Name).(*types.Func); ok {
						ctors = append(ctors, fn)
					}
				}
				return true
			})
			if len(ctors) == 0 {
				bad = append(bad, fmt.Sprintf("%s: catalog entry %s names no constructor of a script package", p.fset.Position(kv.Pos()), path))
			}
			out[path] = ctors
		}
		return false
	})
	if len(bad) > 0 {
		return nil, errors.New("handleops: " + strings.Join(bad, "; "))
	}
	return out, nil
}

func isCatalogType(e ast.Expr, imports map[string]string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Catalog" {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && imports[x.Name] == ScriptPkg
}

// node is a piece of code to visit: a declared function or an expression
// (a function literal, or whatever a hook field is set to).
type node struct {
	n    ast.Node
	info *types.Info
}

// Script returns, per Hooks field the script built by ctors sets ("OnTalk"),
// the hostile-only operations that hook reaches, sorted; a hook that
// reaches none is left out. A call through a function value the scan
// cannot follow (a function-typed parameter, a variable with no seen
// assignment, a function type the content packages make no value of) is an
// error: what it calls is unknown.
func (p *Program) Script(ctors []*types.Func) (map[string][]string, error) {
	roots := p.hookRoots(ctors)
	out := map[string][]string{}
	var unresolved []string
	for field := range roots {
		ops := map[string]bool{}
		p.walk(roots[field], func(nd node, visit func(node), hook func(string)) {
			// anyOf visits every function value whose signature is t's
			// that the content packages make, at a call through a value the
			// scan does not follow. Signatures are compared, not types: a
			// value of an unnamed signature is assignable to a named
			// function type and the other way round.
			anyOf := func(at ast.Node, t types.Type) {
				found := false
				for _, fv := range p.funcValues {
					if types.Identical(fv.sig.Underlying(), t.Underlying()) {
						found = true
						visit(fv.nd)
					}
				}
				if !found {
					unresolved = append(unresolved, fmt.Sprintf("%s: a %s value", p.fset.Position(at.Pos()), t))
				}
			}
			ast.Inspect(nd.n, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					if t, ok := p.dynamicCallee(n.Fun, nd.info); ok {
						anyOf(n.Fun, t)
					}
				case *ast.Ident:
					switch obj := nd.info.Uses[n].(type) {
					case *types.Var:
						if name, ok := p.hooks[obj]; ok {
							hook(name)
						} else if isFuncVar(obj) {
							vals, ok := p.funcVars[obj]
							if !ok && !p.dynVars[obj] {
								unresolved = append(unresolved, p.fset.Position(n.Pos()).String()+": "+n.Name)
							}
							if p.dynVars[obj] {
								anyOf(n, obj.Type())
							}
							for _, v := range vals {
								if t, ok := p.dynamic(v.n.(ast.Expr), v.info); ok {
									anyOf(v.n, t)
								} else {
									visit(v)
								}
							}
						}
					case *types.Func:
						for _, fn := range p.targets(obj.Origin()) {
							switch {
							case fn.Pkg() != nil && fn.Pkg().Path() == ScriptPkg:
								if p.kinded[fn] {
									if name, ok := opName(fn); ok {
										ops[name] = true
									}
								}
								if p.isInvoker(fn) {
									hook("On" + fn.Name())
								}
								for _, h := range p.reentry[fn] {
									hook(h)
								}
							default:
								if d, ok := p.decls[fn]; ok {
									visit(node{d.fn, d.info})
								}
							}
						}
					}
				}
				return true
			})
		}, roots)
		if len(ops) > 0 {
			out[field] = slices.Sorted(maps.Keys(ops))
		}
	}
	if len(unresolved) > 0 {
		slices.Sort(unresolved)
		return nil, fmt.Errorf("handleops: calls through function values of unknown origin: %s", strings.Join(slices.Compact(unresolved), "; "))
	}
	return out, nil
}

// dynamicCallee returns the type of the function value fun, the callee of
// a call, when the scan does not follow it to the functions it may hold.
func (p *Program) dynamicCallee(fun ast.Expr, info *types.Info) (types.Type, bool) {
	if tv, ok := info.Types[fun]; !ok || !tv.IsValue() {
		return nil, false // a conversion
	}
	return p.dynamic(fun, info)
}

// dynamic returns the type of e, an expression of function type, when the
// scan does not follow it to the functions it may hold: everything but a
// function literal, a function or method, a Hooks field and a variable of
// function type (which the walk follows to its assignments).
func (p *Program) dynamic(e ast.Expr, info *types.Info) (types.Type, bool) {
	e = ast.Unparen(e)
	t := info.TypeOf(e)
	if t == nil {
		return nil, false
	}
	if _, ok := t.Underlying().(*types.Signature); !ok {
		return nil, false
	}
	switch x := e.(type) {
	case *ast.FuncLit, *ast.Ident:
		return nil, false
	case *ast.SelectorExpr:
		sel := info.Selections[x]
		if sel == nil || sel.Kind() != types.FieldVal {
			return nil, false // a qualified name, a method value or expression
		}
		if _, ok := p.hooks[sel.Obj().(*types.Var)]; ok {
			return nil, false
		}
	case *ast.IndexExpr:
		if _, ok := info.Uses[selName(x.X)].(*types.Func); ok {
			return nil, false // an instantiated generic function
		}
	case *ast.IndexListExpr:
		if _, ok := info.Uses[selName(x.X)].(*types.Func); ok {
			return nil, false
		}
	}
	return t, true
}

// selName returns the identifier e names: e itself, or a selector's name.
func selName(e ast.Expr) *ast.Ident {
	switch x := ast.Unparen(e).(type) {
	case *ast.Ident:
		return x
	case *ast.SelectorExpr:
		return x.Sel
	}
	return nil
}

// isInvoker reports whether fn is a Hooks invoker: the method of the hook
// field "On"+name.
func (p *Program) isInvoker(fn *types.Func) bool {
	recv := fn.Type().(*types.Signature).Recv()
	if recv == nil {
		return false
	}
	t := recv.Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if named, ok := t.(*types.Named); !ok || named.Obj() != p.hooksType.Obj() {
		return false
	}
	for _, f := range p.hooks {
		if f == "On"+fn.Name() {
			return true
		}
	}
	return false
}

// walk visits, once each, from, everything visit reaches and, for each
// hook named, that hook's roots.
func (p *Program) walk(from []node, each func(nd node, visit func(node), hook func(string)), roots map[string][]node) {
	seen := map[ast.Node]bool{}
	seenHook := map[string]bool{}
	var queue []node
	visit := func(nd node) {
		if !seen[nd.n] {
			seen[nd.n] = true
			queue = append(queue, nd)
		}
	}
	hook := func(name string) {
		if !seenHook[name] {
			seenHook[name] = true
			for _, nd := range roots[name] {
				visit(nd)
			}
		}
	}
	for _, nd := range from {
		visit(nd)
	}
	for len(queue) > 0 {
		nd := queue[0]
		queue = queue[1:]
		each(nd, visit, hook)
	}
}

// hookRoots returns, per Hooks field, every expression the code reachable
// from ctors sets it to: in a Hooks literal or by an assignment.
func (p *Program) hookRoots(ctors []*types.Func) map[string][]node {
	roots := map[string][]node{}
	var from []node
	for _, fn := range ctors {
		if d, ok := p.decls[fn]; ok {
			from = append(from, node{d.fn, d.info})
		}
	}
	p.walk(from, func(nd node, visit func(node), _ func(string)) {
		ast.Inspect(nd.n, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CompositeLit:
				if t, ok := nd.info.Types[n].Type.(*types.Named); ok && t.Obj() == p.hooksType.Obj() {
					for _, elt := range n.Elts {
						if kv, ok := elt.(*ast.KeyValueExpr); ok {
							if name, ok := p.hookField(kv.Key, nd.info); ok {
								roots[name] = append(roots[name], node{kv.Value, nd.info})
							}
						}
					}
				}
			case *ast.AssignStmt:
				for i, lhs := range n.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok && i < len(n.Rhs) {
						if name, ok := p.hookField(sel.Sel, nd.info); ok {
							roots[name] = append(roots[name], node{n.Rhs[i], nd.info})
						}
					}
				}
			case *ast.Ident:
				if fn, ok := nd.info.Uses[n].(*types.Func); ok {
					for _, t := range p.targets(fn.Origin()) {
						if d, ok := p.decls[t]; ok && t.Pkg().Path() != ScriptPkg {
							visit(node{d.fn, d.info})
						}
					}
				}
			}
			return true
		})
	}, nil)
	return roots
}

func (p *Program) hookField(e ast.Expr, info *types.Info) (string, bool) {
	id, ok := e.(*ast.Ident)
	if !ok {
		return "", false
	}
	v, ok := info.Uses[id].(*types.Var)
	if !ok {
		return "", false
	}
	name, ok := p.hooks[v]
	return name, ok
}

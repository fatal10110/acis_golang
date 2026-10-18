package scriptfp

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// goPackage is the fingerprint of a Go script package: the same literal
// classes as a reference fingerprint, the names it calls, and its hooks.
type goPackage struct {
	numbers, strs, chars map[string]bool
	calls                map[string]int // called function or method name -> sites
	hooks                []Hook         // reference hook names (onX for field OnX)
}

// readGoPackage fingerprints the non-test Go files of the package at dir.
func readGoPackage(dir string) (*goPackage, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("scriptfp: read package %s: %w", dir, err)
	}
	pkg := &goPackage{numbers: map[string]bool{}, strs: map[string]bool{}, chars: map[string]bool{}, calls: map[string]int{}}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("scriptfp: parse %s: %w", name, err)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("scriptfp: no Go files in %s", dir)
	}

	funcs := map[string]*ast.FuncDecl{}
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Body != nil {
				funcs[fn.Name.Name] = fn
			}
		}
	}
	var visit func(ast.Node) bool
	visit = func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.ImportSpec:
			return false
		case *ast.BasicLit:
			pkg.literal(n)
		case *ast.CallExpr:
			name := calledName(n)
			if name != "" {
				pkg.calls[name]++
			}
			if name == "Named" {
				// Hooks.Named takes the reference class name, which is no
				// literal of the class: only its receiver is read.
				ast.Inspect(n.Fun, visit)
				return false
			}
		case *ast.CompositeLit:
			if renamed, ok := hooksType(n.Type); ok {
				pkg.hookFields(n, funcs, renamed)
			}
		}
		return true
	}
	for _, file := range files {
		ast.Inspect(file, visit)
	}
	slices.SortStableFunc(pkg.hooks, func(a, b Hook) int { return strings.Compare(a.Name, b.Name) })
	return pkg, nil
}

func (pkg *goPackage) literal(lit *ast.BasicLit) {
	switch lit.Kind {
	case token.INT:
		v, err := strconv.ParseUint(strings.ReplaceAll(lit.Value, "_", ""), 0, 64)
		if err == nil && v > 1 {
			pkg.numbers[strconv.FormatUint(v, 10)] = true
		}
	case token.FLOAT:
		v, err := strconv.ParseFloat(strings.ReplaceAll(lit.Value, "_", ""), 64)
		if s := canonicalFloat(v); err == nil && s != "0" && s != "1" {
			pkg.numbers[s] = true
		}
	case token.STRING:
		if s, err := strconv.Unquote(lit.Value); err == nil && s != "" {
			pkg.strs[s] = true
		}
	case token.CHAR:
		if s, err := strconv.Unquote(lit.Value); err == nil {
			pkg.chars[s] = true
		}
	}
}

// calledName returns the name a call expression calls: the selector of a
// method or qualified call, or a plain function name.
func calledName(call *ast.CallExpr) string {
	fun := call.Fun
	for {
		switch f := fun.(type) {
		case *ast.IndexExpr: // generic instantiation
			fun = f.X
			continue
		case *ast.IndexListExpr:
			fun = f.X
			continue
		case *ast.ParenExpr:
			fun = f.X
			continue
		case *ast.SelectorExpr:
			return f.Sel.Name
		case *ast.Ident:
			return f.Name
		}
		return ""
	}
}

// hooksType reports whether expr is the type of a hooks literal, a script's
// Hooks or a maker's MakerHooks, and returns the reference hook names of
// that type's renamed fields.
func hooksType(expr ast.Expr) (renamed map[string]string, ok bool) {
	var name string
	switch t := expr.(type) {
	case *ast.Ident:
		name = t.Name
	case *ast.SelectorExpr:
		name = t.Sel.Name
	}
	switch name {
	case "Hooks":
		return renamedHooks, true
	case "MakerHooks":
		return renamedMakerHooks, true
	}
	return nil, false
}

// hookFields records each OnX field of a hooks literal with the shape of
// its parent calls: calls to X on any receiver other than the hook's first
// parameter, which is the script or maker itself (a re-entry, not a parent
// call). renamed names the fields whose reference hook is not on plus X.
func (pkg *goPackage) hookFields(lit *ast.CompositeLit, funcs map[string]*ast.FuncDecl, renamed map[string]string) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || !strings.HasPrefix(key.Name, "On") || len(key.Name) < 3 {
			continue
		}
		invoker := strings.TrimPrefix(key.Name, "On")
		var ftype *ast.FuncType
		var body *ast.BlockStmt
		switch v := kv.Value.(type) {
		case *ast.FuncLit:
			ftype, body = v.Type, v.Body
		case *ast.Ident:
			if fn := funcs[v.Name]; fn != nil {
				ftype, body = fn.Type, fn.Body
			}
		}
		name := hookName(invoker, renamed)
		if body == nil {
			pkg.hooks = append(pkg.hooks, Hook{Name: name, Shape: "none"})
			continue
		}
		params := paramNames(ftype)
		shape := "none"
		if direct := parentCalls(body, invoker, params); len(direct) > 0 {
			shape = "direct " + argShape(direct)
		} else {
			for _, callee := range calledFuncs(body) {
				fn := funcs[callee]
				if fn == nil {
					continue
				}
				if via := parentCalls(fn.Body, invoker, paramNames(fn.Type)); len(via) > 0 {
					shape = "helper " + argShape(via)
					break
				}
			}
		}
		pkg.hooks = append(pkg.hooks, Hook{Name: name, Shape: shape})
	}
}

// renamedHooks names the reference hooks whose Go field is not On plus the
// reference name: the bypass-event hook is OnEvent, invoked as Event.
var renamedHooks = map[string]string{"Event": "onAdvEvent"}

// renamedMakerHooks names the reference maker hooks whose MakerHooks field
// is not On plus the reference name.
var renamedMakerHooks = map[string]string{
	"NPCCreated":  "onNpcCreated",
	"NPCDeleted":  "onNpcDeleted",
	"NPCDBInfo":   "onNpcDBInfo",
	"ScriptEvent": "onMakerScriptEvent",
}

// referenceHookName returns the reference hook name of the Hooks field OnX,
// given X.
func referenceHookName(invoker string) string {
	return hookName(invoker, renamedHooks)
}

// hookName returns the reference hook name of the field OnX of a hooks
// type whose renamed fields are renamed, given X.
func hookName(invoker string, renamed map[string]string) string {
	if name, ok := renamed[invoker]; ok {
		return name
	}
	return "on" + invoker
}

func paramNames(ft *ast.FuncType) []string {
	var names []string
	if ft == nil || ft.Params == nil {
		return nil
	}
	for _, field := range ft.Params.List {
		if len(field.Names) == 0 {
			names = append(names, "")
		}
		for _, n := range field.Names {
			names = append(names, n.Name)
		}
	}
	return names
}

// parentCalls returns, for each call <recv>.invoker(...) in body whose
// receiver is not params[0], whether its arguments are params in order.
func parentCalls(body *ast.BlockStmt, invoker string, params []string) []bool {
	var out []bool
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != invoker {
			return true
		}
		if recv, ok := sel.X.(*ast.Ident); ok && len(params) > 0 && recv.Name == params[0] {
			return true
		}
		same := len(call.Args) == len(params)
		for i, arg := range call.Args {
			id, ok := arg.(*ast.Ident)
			same = same && ok && id.Name == params[i] && id.Name != "" && id.Name != "_"
		}
		out = append(out, same)
		return true
	})
	return out
}

// calledFuncs returns the plain function names body calls, in order.
func calledFuncs(body *ast.BlockStmt) []string {
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok {
				out = append(out, id.Name)
			}
		}
		return true
	})
	return out
}

func sortedKeys(m map[string]bool) []string { return slices.Sorted(maps.Keys(m)) }

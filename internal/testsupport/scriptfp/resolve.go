package scriptfp

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ScriptingDir is the reference package that holds the engine base classes,
// relative to the reference source root (aCis_gameserver/java). The script
// trees are its quest, script and task subpackages.
const ScriptingDir = "net/sf/l2j/gameserver/scripting"

var scriptTrees = []string{"quest/", "script/", "task/"}

// tree is the parsed reference source tree with a type index.
type tree struct {
	files  []*javaFile
	byName map[string][]*javaType
}

// loadTree parses every .java file under javaRoot.
func loadTree(javaRoot string) (*tree, error) {
	tr := &tree{byName: map[string][]*javaType{}}
	err := filepath.WalkDir(javaRoot, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(name, ".java") {
			return err
		}
		src, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		f, err := parseJava(string(src))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		rel, _ := filepath.Rel(javaRoot, name)
		rel = filepath.ToSlash(rel)
		if script, ok := strings.CutPrefix(rel, ScriptingDir+"/"); ok {
			for _, prefix := range scriptTrees {
				if strings.HasPrefix(script, prefix) {
					f.key = strings.ReplaceAll(strings.TrimSuffix(script, ".java"), "/", ".")
				}
			}
		}
		tr.files = append(tr.files, f)
		for _, t := range f.types {
			tr.byName[t.name] = append(tr.byName[t.name], t)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scriptfp: load reference tree: %w", err)
	}
	if len(tr.files) == 0 {
		return nil, fmt.Errorf("scriptfp: no Java source under %s", javaRoot)
	}
	for _, list := range tr.byName {
		slices.SortFunc(list, func(a, b *javaType) int { return strings.Compare(a.fqcn(), b.fqcn()) })
	}
	return tr, nil
}

// lookupType resolves a simple type name written in file f inside type ctx.
// It returns nil for types outside the reference tree.
func (tr *tree) lookupType(name string, f *javaFile, ctx *javaType) *javaType {
	cands := tr.byName[name]
	if len(cands) == 0 {
		return nil
	}
	for _, imp := range f.imports {
		if !strings.HasSuffix(imp, "."+name) {
			continue
		}
		for _, cand := range cands {
			if imp == cand.fqcn() {
				return cand
			}
		}
		return nil // an imported library type
	}
	if len(cands) == 1 {
		return cands[0]
	}
	// Nested in the context type or one of its enclosing types or supertypes.
	for c := ctx; c != nil; c = c.outer {
		for _, sup := range tr.supertypes(c) {
			for _, cand := range cands {
				if cand.outer == sup {
					return cand
				}
			}
		}
	}
	for _, cand := range cands {
		if cand.file == f {
			return cand
		}
	}
	for _, imp := range f.imports {
		for _, cand := range cands {
			if imp == cand.fqcn() {
				return cand
			}
		}
	}
	for _, cand := range cands {
		if cand.outer == nil && cand.file.pkg == f.pkg {
			return cand
		}
	}
	for _, imp := range f.imports {
		pkg, ok := strings.CutSuffix(imp, ".*")
		if !ok {
			continue
		}
		for _, cand := range cands {
			if cand.outer == nil && cand.file.pkg == pkg || cand.outer != nil && cand.outer.fqcn() == pkg {
				return cand
			}
		}
	}
	return nil
}

// lookupQualified resolves a declared type, following its qualifier.
func (tr *tree) lookupQualified(ref typeRef, decl *javaType) *javaType {
	if ref.qual == "" {
		return tr.lookupType(ref.name, decl.file, decl)
	}
	first, _, _ := strings.Cut(ref.qual, ".")
	if first[0] >= 'a' && first[0] <= 'z' {
		for _, cand := range tr.byName[ref.name] {
			if cand.fqcn() == ref.qual+"."+ref.name {
				return cand
			}
		}
		return nil
	}
	last := ref.qual[strings.LastIndexByte(ref.qual, '.')+1:]
	outer := tr.lookupType(last, decl.file, decl)
	if outer == nil {
		return nil
	}
	for _, cand := range tr.byName[ref.name] {
		if cand.outer == outer {
			return cand
		}
	}
	return nil
}

// libraryBase returns the first library class t's class chain extends,
// with its type arguments, and the reference class that extends it.
func (tr *tree) libraryBase(t *javaType) (xtype, *javaType) {
	for c := t; c != nil; c = tr.parent(c) {
		if c.extends.name != "" && tr.parent(c) == nil {
			return tr.resolveRef(c.extends, c, nil), c
		}
	}
	return xtype{}, nil
}

// supertypes returns t and every reference type it extends or implements,
// classes first.
func (tr *tree) supertypes(t *javaType) []*javaType {
	var out []*javaType
	seen := map[*javaType]bool{}
	walk := func(c *javaType) {
		for ; c != nil && !seen[c]; c = tr.parent(c) {
			seen[c] = true
			out = append(out, c)
		}
	}
	walk(t)
	for i := 0; i < len(out); i++ {
		for _, ref := range out[i].implements {
			if it := tr.lookupType(ref.name, out[i].file, out[i]); it != nil && !seen[it] {
				walk(it)
			}
		}
	}
	return out
}

// parent returns the reference class t extends, or nil.
func (tr *tree) parent(t *javaType) *javaType {
	if t.extends.name == "" {
		return nil
	}
	return tr.lookupType(t.extends.name, t.file, t.outer)
}

// externalChain reports whether t inherits from a type outside the
// reference tree, so a method missing from the tree may come from there.
func (tr *tree) externalChain(t *javaType) bool {
	for _, c := range tr.supertypes(t) {
		if c.enum || c.extends.name != "" && tr.parent(c) == nil {
			return true
		}
		for _, ref := range c.implements {
			if tr.lookupType(ref.name, c.file, c) == nil {
				return true
			}
		}
	}
	return false
}

var objectMethods = map[string]bool{
	"equals": true, "hashCode": true, "toString": true, "getClass": true, "notify": true,
	"notifyAll": true, "wait": true, "clone": true, "finalize": true,
}

// findMethod returns the type declaring name callable with argc arguments
// on t (argc < 0: any), searching t's supertypes in order.
func (tr *tree) findMethod(t *javaType, name string, argc int) (*javaType, *javaMethod) {
	var byName *javaMethod
	var byNameOwner *javaType
	for _, c := range tr.supertypes(t) {
		for _, m := range c.methods {
			if m.pseudo || m.ctor || m.name != name {
				continue
			}
			if argc < 0 || len(m.params) == argc || m.varargs && argc >= len(m.params)-1 {
				return c, m
			}
			if byName == nil {
				byName, byNameOwner = m, c
			}
		}
	}
	return byNameOwner, byName
}

// findField returns the declared type of field name on t and the type
// declaring it.
func (tr *tree) findField(t *javaType, name string) (typeRef, *javaType, bool) {
	for _, c := range tr.supertypes(t) {
		if ref, ok := c.fields[name]; ok {
			return ref, c, true
		}
	}
	return typeRef{}, nil, false
}

// libraryValue names the unknown type of a value a library call returns.
const libraryValue = "<library>"

// xtype is the static type of an expression.
type xtype struct {
	name   string    // simple name; "" when unknown
	dims   int       // array dimensions
	t      *javaType // the reference type; nil when outside the tree or unknown
	static bool      // a type name, not a value
	super  bool      // the "super" receiver
	args   []xtype   // type arguments, kept for library containers
}

func (x xtype) known() bool { return x.name != "" || x.super }

// resolveRef turns a declared type into an xtype, resolving names in the
// declaring type's file. Type variables are unknown.
func (tr *tree) resolveRef(ref typeRef, decl *javaType, m *javaMethod) xtype {
	if ref.name == "" || m != nil && slices.Contains(m.typeParams, ref.name) {
		return xtype{}
	}
	for c := decl; c != nil; c = c.outer {
		if slices.Contains(c.typeParams, ref.name) {
			return xtype{}
		}
	}
	x := xtype{name: ref.name, dims: ref.dims, t: tr.lookupQualified(ref, decl)}
	for _, arg := range ref.args {
		x.args = append(x.args, tr.resolveRef(arg, decl, m))
	}
	return x
}

// Library containers whose element types the census follows, so a call on
// an element of a List<Player> still names Player.
var (
	elementContainers = map[string]bool{
		"List": true, "ArrayList": true, "LinkedList": true, "CopyOnWriteArrayList": true, "Set": true,
		"HashSet": true, "LinkedHashSet": true, "TreeSet": true, "Collection": true, "Iterable": true,
		"Queue": true, "Deque": true, "ArrayDeque": true, "ConcurrentLinkedQueue": true, "Iterator": true,
		"ListIterator": true, "Stream": true, "Optional": true, "KeySetView": true,
	}
	mapContainers = map[string]bool{
		"Map": true, "HashMap": true, "LinkedHashMap": true, "TreeMap": true, "ConcurrentHashMap": true,
		"EnumMap": true, "Entry": true,
	}
	elementGetters = map[string]bool{
		"get": true, "getFirst": true, "getLast": true, "next": true, "previous": true, "poll": true,
		"pop": true, "removeFirst": true, "removeLast": true, "pollFirst": true, "pollLast": true,
		"peekFirst": true, "peekLast": true, "first": true, "last": true, "element": true, "orElse": true,
		"orElseThrow": true, "orElseGet": true,
	}
	// lambdaElementCalls take a lambda over the receiver's elements.
	lambdaElementCalls = map[string]bool{
		"forEach": true, "forEachOrdered": true, "removeIf": true, "filter": true, "anyMatch": true,
		"allMatch": true, "noneMatch": true, "map": true, "mapToInt": true, "flatMap": true, "ifPresent": true,
		"peek": true,
	}
)

// libraryResult returns the type a library container call yields: an
// element, a key or value, or a container of the same elements.
func libraryResult(recv xtype, name string) xtype {
	if recv.dims > 0 {
		return xtype{}
	}
	switch {
	case elementContainers[recv.name] && len(recv.args) == 1:
		elem := recv.args[0]
		switch {
		case recv.name == "Stream" && (name == "filter" || name == "sorted" || name == "distinct" || name == "limit" || name == "skip" || name == "peek"):
			return recv
		case name == "peek" || elementGetters[name]:
			return elem
		case name == "iterator" || name == "listIterator":
			return xtype{name: "Iterator", args: []xtype{elem}}
		case name == "stream" || name == "parallelStream":
			return xtype{name: "Stream", args: []xtype{elem}}
		case name == "findFirst" || name == "findAny" || name == "min" || name == "max":
			return xtype{name: "Optional", args: []xtype{elem}}
		case name == "toList":
			return xtype{name: "List", args: []xtype{elem}}
		}
	case mapContainers[recv.name] && len(recv.args) == 2:
		key, value := recv.args[0], recv.args[1]
		switch name {
		case "getKey":
			return key
		case "getValue", "get", "remove", "getOrDefault", "computeIfAbsent", "computeIfPresent", "compute", "merge", "put", "putIfAbsent":
			return value
		case "keySet":
			return xtype{name: "Set", args: []xtype{key}}
		case "values":
			return xtype{name: "Collection", args: []xtype{value}}
		case "entrySet":
			return xtype{name: "Set", args: []xtype{{name: "Entry", args: []xtype{key, value}}}}
		}
	}
	return xtype{}
}

// lambdaParam returns the type of parameter index of a lambda passed to a
// library call named name on recv.
func lambdaParam(recv xtype, name string, index int) xtype {
	if recv.dims > 0 || !lambdaElementCalls[name] {
		return xtype{}
	}
	switch {
	case elementContainers[recv.name] && len(recv.args) == 1 && index == 0:
		return recv.args[0]
	case mapContainers[recv.name] && len(recv.args) == 2 && name == "forEach" && index < 2:
		return recv.args[index]
	}
	return xtype{}
}

// callKind classifies a call site.
type callKind uint8

const (
	callOwn        callKind = iota + 1 // declared in a script tree class
	callEngine                         // declared in a reference class outside the script trees
	callExternal                       // a JDK or library type, or a value of one
	callUnresolved                     // the receiver or the method was not found
)

type call struct {
	kind  callKind
	owner *javaType
	name  string // method name; "new" for a constructor
	ret   xtype
}

// row is the census row name of an engine or unresolved call.
func (c call) row() string {
	if c.kind == callUnresolved {
		return "?." + c.name
	}
	return c.owner.qualified() + "." + c.name
}

type localDecl struct {
	pos  int
	name string
	typ  typeRef
	// lambda parameters: the call taking the lambda and the parameter's
	// position; typ is unused.
	lambdaCall, lambdaIndex int
}

// body analyses one method body of type t in file f.
type body struct {
	tr    *tree
	f     *javaFile
	t     *javaType
	m     *javaMethod
	open  []int // for each closing bracket token, the index of its opener
	decls []localDecl
	calls map[int]call // memo by name token index
	depth int
}

func newBody(tr *tree, f *javaFile, t *javaType, m *javaMethod, open []int) *body {
	b := &body{tr: tr, f: f, t: t, m: m, open: open, calls: map[int]call{}}
	p := &javaParser{f: f}
	for i := m.body[0]; i < m.body[1]; i++ {
		prev := p.tok(i - 1)
		if p.tok(i).is("->") {
			b.lambdaParams(i)
			continue
		}
		if !(prev.is("{") || prev.is("}") || prev.is(";") || prev.is("(") || prev.is(":") ||
			prev.is("->") || prev.ident("final") || prev.ident("instanceof")) {
			continue
		}
		typ, next, ok := p.parseType(i)
		if !ok || typ.name == "var" {
			continue
		}
		name := p.tok(next)
		if name.kind != tokIdent || javaKeywords[name.text] {
			continue
		}
		after := p.tok(next + 1)
		if prev.ident("instanceof") || after.is("=") || after.is(";") || after.is(",") || after.is(":") ||
			after.is(")") || after.is("[") {
			b.decls = append(b.decls, localDecl{pos: next, name: name.text, typ: typ})
		}
	}
	slices.SortStableFunc(b.decls, func(x, y localDecl) int { return x.pos - y.pos })
	return b
}

// lambdaParams records the untyped parameters of the lambda whose arrow is
// at i when the lambda is the first argument of a call: x -> or (x, y) ->.
func (b *body) lambdaParams(arrow int) {
	var names []int
	start := arrow - 1
	switch {
	case b.tok(start).kind == tokIdent:
		names = []int{start}
	case b.tok(start).is(")"):
		start = b.open[start]
		for k := start + 1; k < arrow-1; k += 2 {
			if b.tok(k).kind != tokIdent || !(b.tok(k+1).is(",") || k+1 == arrow-1) {
				return // typed or empty parameters
			}
			names = append(names, k)
		}
	default:
		return
	}
	if !b.tok(start-1).is("(") || b.tok(start-2).kind != tokIdent || !b.tok(start-3).is(".") {
		return
	}
	for index, k := range names {
		b.decls = append(b.decls, localDecl{pos: k, name: b.tok(k).text, lambdaCall: start - 2, lambdaIndex: index})
	}
}

func (b *body) tok(i int) javaToken {
	if i < 0 || i >= len(b.f.toks) {
		return javaToken{}
	}
	return b.f.toks[i]
}

// variable returns the type of the plain identifier name used at pos.
func (b *body) variable(name string, pos int) (xtype, bool) {
	for k := len(b.decls) - 1; k >= 0; k-- {
		if d := b.decls[k]; d.pos < pos && d.name == name {
			if d.lambdaCall > 0 {
				return lambdaParam(b.typeEnding(d.lambdaCall-2), b.tok(d.lambdaCall).text, d.lambdaIndex), true
			}
			return b.tr.resolveRef(d.typ, b.t, b.m), true
		}
	}
	for _, p := range b.m.params {
		if p.name == name {
			return b.tr.resolveRef(p.typ, b.t, b.m), true
		}
	}
	for c := b.t; c != nil; c = c.outer {
		if ref, decl, ok := b.tr.findField(c, name); ok {
			return b.tr.resolveRef(ref, decl, nil), true
		}
	}
	return xtype{}, false
}

// typeEnding returns the type of the expression whose last token is at j.
func (b *body) typeEnding(j int) xtype {
	if b.depth > 64 {
		return xtype{}
	}
	b.depth++
	defer func() { b.depth-- }()

	t := b.tok(j)
	switch {
	case t.ident("this"):
		return xtype{name: b.t.name, t: b.t}
	case t.ident("super"):
		return xtype{super: true}
	case t.kind == tokString:
		return xtype{name: "String"}
	case t.kind == tokIdent && !javaKeywords[t.text]:
		upper := t.text[0] >= 'A' && t.text[0] <= 'Z'
		if b.tok(j - 1).is(".") {
			base := b.typeEnding(j - 2)
			if base.t != nil {
				if base.dims == 0 {
					if ref, decl, ok := b.tr.findField(base.t, t.text); ok {
						return b.tr.resolveRef(ref, decl, nil)
					}
				}
				if nested := b.tr.lookupType(t.text, base.t.file, base.t); nested != nil && upper {
					return xtype{name: t.text, t: nested, static: true}
				}
				return xtype{}
			}
			if base.dims > 0 && t.text == "length" {
				return xtype{name: "int"}
			}
			if !base.known() && upper {
				return xtype{name: t.text, t: b.tr.lookupType(t.text, b.f, b.t), static: true}
			}
			if base.static && upper { // a library type's nested type
				return xtype{name: t.text, static: true}
			}
			return xtype{}
		}
		if x, ok := b.variable(t.text, j); ok {
			return x
		}
		if upper {
			return xtype{name: t.text, t: b.tr.lookupType(t.text, b.f, b.t), static: true}
		}
		return xtype{}
	case t.is(")"):
		k := b.open[j]
		prev := b.tok(k - 1)
		if prev.kind == tokIdent && !javaKeywords[prev.text] {
			if ctor, ok := b.constructed(k - 1); ok {
				return ctor
			}
			return b.callAt(k - 1).ret
		}
		if prev.is(">") {
			if ctor, ok := b.constructed(k - 1); ok {
				return ctor
			}
			return xtype{}
		}
		// A parenthesized expression or a cast: ((Type) x).
		if b.tok(k + 1).is("(") {
			p := &javaParser{f: b.f}
			if typ, next, ok := p.parseType(k + 2); ok && next == b.f.match[k+1] {
				return b.tr.resolveRef(typ, b.t, b.m)
			}
		}
		return b.typeEnding(j - 1)
	case t.is("]"):
		base := b.typeEnding(b.open[j] - 1)
		if base.dims > 0 {
			base.dims--
			return base
		}
		return xtype{}
	}
	return xtype{}
}

// constructed returns the type created when the tokens ending at j are the
// type of a "new" expression.
func (b *body) constructed(j int) (xtype, bool) {
	i := j
	if b.tok(i).is(">") {
		depth := 0
		for ; i > 0; i-- {
			if b.tok(i).is(">") {
				depth++
			} else if b.tok(i).is("<") {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		i--
	}
	name := b.tok(i)
	if name.kind != tokIdent {
		return xtype{}, false
	}
	k := i - 1
	for b.tok(k).is(".") && b.tok(k-1).kind == tokIdent {
		k -= 2
	}
	if !b.tok(k).ident("new") {
		return xtype{}, false
	}
	return xtype{name: name.text, t: b.tr.lookupType(name.text, b.f, b.t)}, true
}

// argc counts the arguments of the call whose '(' is at i.
func (b *body) argc(i int) int {
	end := b.f.match[i]
	if end == i+1 {
		return 0
	}
	n := 1
	for k := i + 1; k < end; k++ {
		switch t := b.tok(k); {
		case t.is("(") || t.is("[") || t.is("{"):
			k = b.f.match[k]
		case t.is(","):
			n++
		}
	}
	return n
}

// classify finds the declaration of method name for receiver recv.
func (b *body) classify(recv xtype, name string, argc int) call {
	c := call{name: name}
	var owner *javaType
	var m *javaMethod
	switch {
	case recv.super:
		if parent := b.tr.parent(b.t); parent != nil {
			owner, m = b.tr.findMethod(parent, name, argc)
		}
	case recv.t != nil && recv.dims == 0:
		owner, m = b.tr.findMethod(recv.t, name, argc)
		if m != nil || objectMethods[name] {
			break
		}
		if base, via := b.tr.libraryBase(recv.t); via != nil {
			// Inherited from a library class: a script-facing call on the
			// reference class that extends it.
			c.kind, c.owner = callEngine, via
			c.ret = libraryResult(base, name)
			if !c.ret.known() {
				c.ret = xtype{name: libraryValue}
			}
			return c
		}
		if b.tr.externalChain(recv.t) {
			c.kind = callExternal
			c.ret = xtype{name: libraryValue}
			return c
		}
	case recv.name != "":
		c.kind = callExternal
		c.ret = libraryResult(recv, name)
		if !c.ret.known() {
			// A library call's result whose type is not followed is taken
			// as a library value too.
			c.ret = xtype{name: libraryValue}
		}
		return c
	}
	if m == nil {
		c.kind = callUnresolved
		if objectMethods[name] {
			c.kind = callExternal
		}
		return c
	}
	return b.declared(owner, m, c)
}

func (b *body) declared(owner *javaType, m *javaMethod, c call) call {
	c.owner = owner
	c.ret = b.tr.resolveRef(m.ret, owner, m)
	if owner.file.key != "" {
		c.kind = callOwn
	} else {
		c.kind = callEngine
	}
	return c
}

// callAt classifies the method call whose name token is at i.
func (b *body) callAt(i int) call {
	if c, ok := b.calls[i]; ok {
		return c
	}
	name := b.tok(i).text
	argc := -1
	if b.tok(i + 1).is("(") {
		argc = b.argc(i + 1)
	}
	var c call
	switch prev := b.tok(i - 1); {
	case prev.is(".") || prev.is("::"):
		c = b.classify(b.typeEnding(i-2), name, argc)
	default:
		c = call{name: name, kind: callUnresolved}
		for t := b.t; t != nil; t = t.outer {
			if owner, m := b.tr.findMethod(t, name, argc); m != nil {
				c = b.declared(owner, m, c)
				break
			}
			if objectMethods[name] {
				c.kind = callExternal
				break
			}
		}
	}
	b.calls[i] = c
	return c
}

// sites returns every call made in the body, constructors included and
// parent calls (super.name) left out: the hook shape records those.
func (b *body) sites() []call {
	var out []call
	for i := b.m.body[0]; i < b.m.body[1]; i++ {
		t := b.tok(i)
		next := b.tok(i + 1)
		switch {
		case t.ident("new") && b.tok(i+1).kind == tokIdent:
			p := &javaParser{f: b.f}
			typ, end, ok := p.parseType(i + 1)
			if !ok || !b.tok(end).is("(") {
				continue
			}
			c := call{name: "new", kind: callExternal}
			if ct := b.tr.lookupType(typ.name, b.f, b.t); ct != nil {
				c.owner = ct
				c.kind = callEngine
				if ct.file.key != "" {
					c.kind = callOwn
				}
			}
			out = append(out, c)
			i = end // the type name is not a method call
		case t.kind != tokIdent || javaKeywords[t.text] && !t.ident("new"):
		case b.tok(i - 1).is("::"):
			if t.ident("new") {
				continue
			}
			out = append(out, b.callAt(i))
		case next.is("(") && !t.ident("new"):
			prev := b.tok(i - 1)
			if prev.kind == tokIdent && (!javaKeywords[prev.text] || javaPrimitives[prev.text]) || prev.is("]") {
				continue // a method declared in an anonymous class body
			}
			if prev.is(".") && b.tok(i-2).ident("super") {
				continue
			}
			out = append(out, b.callAt(i))
		}
	}
	return out
}

// superCalls returns, for each "super.name(" in the body, whether the
// arguments are exactly the method's parameters in order.
func (b *body) superCalls(name string) []bool {
	var out []bool
	for i := b.m.body[0]; i+3 < b.m.body[1]; i++ {
		if !b.tok(i).ident("super") || !b.tok(i+1).is(".") || !b.tok(i+2).ident(name) || !b.tok(i+3).is("(") {
			continue
		}
		open := i + 3
		end := b.f.match[open]
		var args []string
		start := open + 1
		for k := start; k <= end; k++ {
			switch t := b.tok(k); {
			case k == end || t.is(","):
				if k > start || k != end {
					arg := ""
					if k == start+1 && b.tok(start).kind == tokIdent {
						arg = b.tok(start).text
					}
					args = append(args, arg)
				}
				start = k + 1
			case t.is("(") || t.is("[") || t.is("{"):
				k = b.f.match[k]
			}
		}
		same := len(args) == len(b.m.params)
		for k, arg := range args {
			same = same && arg != "" && arg == b.m.params[k].name
		}
		out = append(out, same)
	}
	return out
}

// closers builds, for each closing bracket, the index of its opener.
func closers(f *javaFile) []int {
	open := make([]int, len(f.toks))
	for i, t := range f.toks {
		if t.kind == tokOp && (t.text == "(" || t.text == "[" || t.text == "{") {
			open[f.match[i]] = i
		}
	}
	return open
}

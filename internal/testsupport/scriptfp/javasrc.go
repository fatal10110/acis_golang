package scriptfp

import (
	"fmt"
	"strings"
)

// javaFile is the declaration skeleton of one Java source file: enough to
// type method receivers and find hooks, not a full syntax tree.
type javaFile struct {
	key     string // reference script key for files under the script trees, else ""
	pkg     string
	imports []string
	types   []*javaType // every type declared in the file, outer before inner
	toks    []javaToken
	match   []int // for each opening bracket token, the index of its closer
}

type javaType struct {
	name       string // simple name
	outer      *javaType
	file       *javaFile
	enum       bool
	extends    typeRef
	implements []typeRef
	typeParams []string
	fields     map[string]typeRef
	methods    []*javaMethod
}

// qualified returns the type's name with its enclosing types, such as
// "Outer.Inner".
func (t *javaType) qualified() string {
	if t.outer == nil {
		return t.name
	}
	return t.outer.qualified() + "." + t.name
}

func (t *javaType) fqcn() string {
	if t.outer != nil {
		return t.outer.fqcn() + "." + t.name
	}
	if t.file.pkg == "" {
		return t.name
	}
	return t.file.pkg + "." + t.name
}

// typeRef is a type as written: its last simple name with generic arguments
// dropped, and its array dimensions.
type typeRef struct {
	name string
	qual string // the qualifier written before name, such as "Map" in Map.Entry
	dims int
	args []typeRef // type arguments of the last name; a wildcard is its bound or unknown
}

type javaParam struct {
	typ  typeRef
	name string
}

type javaMethod struct {
	name       string
	ret        typeRef
	params     []javaParam
	varargs    bool
	static     bool
	private    bool
	ctor       bool
	pseudo     bool // an initializer block or field initializer, not a declared method
	typeParams []string
	body       [2]int // token range inside the braces
	hasBody    bool   // false for an abstract or interface method
}

var javaKeywords = map[string]bool{
	"abstract": true, "assert": true, "boolean": true, "break": true, "byte": true, "case": true,
	"catch": true, "char": true, "class": true, "const": true, "continue": true, "default": true,
	"do": true, "double": true, "else": true, "enum": true, "extends": true, "final": true,
	"finally": true, "float": true, "for": true, "goto": true, "if": true, "implements": true,
	"import": true, "instanceof": true, "int": true, "interface": true, "long": true, "native": true,
	"new": true, "package": true, "private": true, "protected": true, "public": true, "return": true,
	"short": true, "static": true, "strictfp": true, "super": true, "switch": true, "synchronized": true,
	"this": true, "throw": true, "throws": true, "transient": true, "try": true, "void": true,
	"volatile": true, "while": true, "true": true, "false": true, "null": true, "yield": true,
}

var javaPrimitives = map[string]bool{
	"boolean": true, "byte": true, "char": true, "double": true, "float": true, "int": true,
	"long": true, "short": true, "void": true,
}

var javaModifiers = map[string]bool{
	"public": true, "protected": true, "private": true, "static": true, "final": true,
	"abstract": true, "synchronized": true, "native": true, "transient": true, "volatile": true,
	"strictfp": true, "default": true, "sealed": true,
}

// parseJava lexes and parses one source file.
func parseJava(src string) (*javaFile, error) {
	toks, err := lexJava(src)
	if err != nil {
		return nil, err
	}
	f := &javaFile{toks: toks, match: make([]int, len(toks))}
	var stack []int
	for i, t := range toks {
		if t.kind != tokOp {
			continue
		}
		switch t.text {
		case "(", "[", "{":
			stack = append(stack, i)
		case ")", "]", "}":
			if len(stack) == 0 {
				return nil, fmt.Errorf("line %d: unbalanced %q", t.line, t.text)
			}
			open := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if want := map[string]string{"(": ")", "[": "]", "{": "}"}[toks[open].text]; want != t.text {
				return nil, fmt.Errorf("line %d: %q closes %q of line %d", t.line, t.text, toks[open].text, toks[open].line)
			}
			f.match[open] = i
		}
	}
	if len(stack) > 0 {
		return nil, fmt.Errorf("line %d: unclosed %q", toks[stack[0]].line, toks[stack[0]].text)
	}

	p := &javaParser{f: f}
	for i := 0; i < len(toks); {
		switch {
		case toks[i].ident("package"):
			name, next := p.dotted(i + 1)
			f.pkg, i = name, next+1
		case toks[i].ident("import"):
			j := i + 1
			if toks[j].ident("static") {
				j++
			}
			name, next := p.dotted(j)
			if next < len(toks) && toks[next].is(".") && toks[next+1].is("*") {
				name += ".*"
				next += 2
			}
			f.imports = append(f.imports, name)
			i = next + 1
		case toks[i].is(";"):
			i++
		default:
			next, err := p.member(i, nil)
			if err != nil {
				return nil, err
			}
			i = next
		}
	}
	return f, nil
}

type javaParser struct {
	f *javaFile
}

func (p *javaParser) tok(i int) javaToken {
	if i < 0 || i >= len(p.f.toks) {
		return javaToken{}
	}
	return p.f.toks[i]
}

// dotted reads a.b.c starting at i and returns it with the index after it.
func (p *javaParser) dotted(i int) (string, int) {
	var parts []string
	for p.tok(i).kind == tokIdent {
		parts = append(parts, p.tok(i).text)
		if !p.tok(i+1).is(".") || p.tok(i+2).kind != tokIdent {
			i++
			break
		}
		i += 2
	}
	return strings.Join(parts, "."), i
}

// skipAnnotation returns the index after the annotation at i ('@').
func (p *javaParser) skipAnnotation(i int) int {
	_, i = p.dotted(i + 1)
	if p.tok(i).is("(") {
		i = p.f.match[i] + 1
	}
	return i
}

// skipAngles returns the index after the type argument or parameter list
// opening at i, or -1 when the tokens there cannot be one.
func (p *javaParser) skipAngles(i int) int {
	depth := 0
	for ; i < len(p.f.toks); i++ {
		t := p.tok(i)
		switch {
		case t.is("<"):
			depth++
		case t.is(">"):
			depth--
			if depth == 0 {
				return i + 1
			}
		case t.kind == tokIdent && (!javaKeywords[t.text] || javaPrimitives[t.text] || t.text == "extends" || t.text == "super"):
		case t.is(".") || t.is(",") || t.is("?") || t.is("&") || t.is("[") || t.is("]") || t.is("@"):
		default:
			return -1
		}
	}
	return -1
}

// typeParams returns the names declared by the type parameter list at i.
func (p *javaParser) typeParams(i, end int) []string {
	var names []string
	depth := 0
	for j := i; j < end; j++ {
		switch t := p.tok(j); {
		case t.is("<"):
			depth++
			if depth == 1 && p.tok(j+1).kind == tokIdent {
				names = append(names, p.tok(j+1).text)
			}
		case t.is(">"):
			depth--
		case t.is(",") && depth == 1 && p.tok(j+1).kind == tokIdent:
			names = append(names, p.tok(j+1).text)
		}
	}
	return names
}

// parseType reads a type at i and returns it with the index after it, or
// ok false when no type starts there.
func (p *javaParser) parseType(i int) (typeRef, int, bool) {
	t := p.tok(i)
	if t.kind != tokIdent || javaKeywords[t.text] && !javaPrimitives[t.text] {
		return typeRef{}, i, false
	}
	ref := typeRef{name: t.text}
	i++
	for {
		switch {
		case p.tok(i).is(".") && p.tok(i+1).kind == tokIdent && !javaKeywords[p.tok(i+1).text]:
			if ref.qual != "" {
				ref.qual += "."
			}
			ref.qual += ref.name
			ref.name = p.tok(i + 1).text
			ref.args = nil
			i += 2
		case p.tok(i).is("<"):
			if p.skipAngles(i) < 0 {
				return typeRef{}, i, false
			}
			args, next, ok := p.typeArgs(i)
			if !ok {
				return typeRef{}, i, false
			}
			ref.args = args
			i = next
		case p.tok(i).is("[") && p.tok(i+1).is("]"):
			ref.dims++
			i += 2
		default:
			return ref, i, true
		}
	}
}

// typeArgs parses the type argument list opening at i.
func (p *javaParser) typeArgs(i int) ([]typeRef, int, bool) {
	i++
	if p.tok(i).is(">") {
		return nil, i + 1, true
	}
	var args []typeRef
	for {
		var arg typeRef
		if p.tok(i).is("?") {
			i++
			if p.tok(i).ident("extends") || p.tok(i).ident("super") {
				bound, next, ok := p.parseType(i + 1)
				if !ok {
					return nil, i, false
				}
				if p.tok(i).ident("extends") {
					arg = bound
				}
				i = next
			}
		} else {
			ref, next, ok := p.parseType(i)
			if !ok {
				return nil, i, false
			}
			arg, i = ref, next
		}
		for p.tok(i).is("&") { // intersection bound
			_, next, ok := p.parseType(i + 1)
			if !ok {
				return nil, i, false
			}
			i = next
		}
		args = append(args, arg)
		switch {
		case p.tok(i).is(","):
			i++
		case p.tok(i).is(">"):
			return args, i + 1, true
		default:
			return nil, i, false
		}
	}
}

// member parses one type member (or top-level declaration) at i for owner
// and returns the index after it.
func (p *javaParser) member(i int, owner *javaType) (int, error) {
	var static, private bool
	var typeParams []string
	for {
		t := p.tok(i)
		switch {
		case i >= len(p.f.toks):
			return i, nil
		case t.is("@") && p.tok(i+1).ident("interface"):
			return p.typeDecl(i+1, owner)
		case t.is("@"):
			i = p.skipAnnotation(i)
			continue
		case t.kind == tokIdent && javaModifiers[t.text] && !(t.text == "default" && p.tok(i+1).is(":")):
			static = static || t.text == "static"
			private = private || t.text == "private"
			i++
			continue
		case t.ident("non") && p.tok(i+1).is("-") && p.tok(i+2).ident("sealed"):
			i += 3
			continue
		case t.ident("class") || t.ident("interface") || t.ident("enum") ||
			t.ident("record") && p.tok(i+1).kind == tokIdent && (p.tok(i+2).is("(") || p.tok(i+2).is("<")):
			return p.typeDecl(i, owner)
		case t.is(";"):
			return i + 1, nil
		case t.is("{") && owner != nil:
			end := p.f.match[i]
			owner.methods = append(owner.methods, &javaMethod{name: "<init>", static: static, pseudo: true, body: [2]int{i + 1, end}})
			return end + 1, nil
		case t.is("<"):
			next := p.skipAngles(i)
			if next < 0 {
				return 0, fmt.Errorf("line %d: bad type parameters", t.line)
			}
			typeParams = p.typeParams(i, next)
			i = next
			continue
		}
		break
	}
	if owner == nil {
		return 0, fmt.Errorf("line %d: unexpected %q at top level", p.tok(i).line, p.tok(i).text)
	}

	m := &javaMethod{static: static, private: private, typeParams: typeParams}
	if p.tok(i).ident(owner.name) && p.tok(i+1).is("{") { // compact record constructor
		m.name, m.ctor = owner.name, true
		m.body, m.hasBody = [2]int{i + 2, p.f.match[i+1]}, true
		owner.methods = append(owner.methods, m)
		return p.f.match[i+1] + 1, nil
	}
	if p.tok(i).kind == tokIdent && p.tok(i+1).is("(") {
		if p.tok(i).text != owner.name {
			return 0, fmt.Errorf("line %d: method %s without a return type", p.tok(i).line, p.tok(i).text)
		}
		m.name, m.ctor = p.tok(i).text, true
		return p.methodRest(i+1, owner, m)
	}
	typ, next, ok := p.parseType(i)
	if !ok || p.tok(next).kind != tokIdent {
		return 0, fmt.Errorf("line %d: cannot parse member at %q", p.tok(i).line, p.tok(i).text)
	}
	i = next
	if p.tok(i + 1).is("(") {
		m.name, m.ret = p.tok(i).text, typ
		return p.methodRest(i+1, owner, m)
	}
	// Fields: name [dims] [= init] {, name [dims] [= init]} ;
	for {
		name := p.tok(i).text
		ref := typ
		i++
		for p.tok(i).is("[") && p.tok(i+1).is("]") {
			ref.dims++
			i += 2
		}
		owner.fields[name] = ref
		if p.tok(i).is("=") {
			start := i + 1
			i = p.skipExpr(start)
			owner.methods = append(owner.methods, &javaMethod{name: "<init>", static: static, pseudo: true, body: [2]int{start, i}})
		}
		switch {
		case p.tok(i).is(";"):
			return i + 1, nil
		case p.tok(i).is(",") && p.tok(i+1).kind == tokIdent:
			i++
		default:
			return 0, fmt.Errorf("line %d: unexpected %q in field %s", p.tok(i).line, p.tok(i).text, name)
		}
	}
}

// skipExpr returns the index of the ',' or ';' ending the expression at i.
func (p *javaParser) skipExpr(i int) int {
	for i < len(p.f.toks) {
		t := p.tok(i)
		switch {
		case t.is("(") || t.is("[") || t.is("{"):
			i = p.f.match[i] + 1
		case t.is(",") || t.is(";"):
			return i
		default:
			i++
		}
	}
	return i
}

// methodRest parses parameters, throws clause and body from the '(' at i.
func (p *javaParser) methodRest(i int, owner *javaType, m *javaMethod) (int, error) {
	end := p.f.match[i]
	m.params, m.varargs = p.params(i+1, end)
	i = end + 1
	for p.tok(i).is("[") && p.tok(i+1).is("]") {
		m.ret.dims++
		i += 2
	}
	for i < len(p.f.toks) && !p.tok(i).is("{") && !p.tok(i).is(";") {
		i++
	}
	if p.tok(i).is("{") {
		m.body, m.hasBody = [2]int{i + 1, p.f.match[i]}, true
		i = p.f.match[i]
	}
	owner.methods = append(owner.methods, m)
	return i + 1, nil
}

// params parses the formal parameters in the token range [i, end).
func (p *javaParser) params(i, end int) ([]javaParam, bool) {
	var out []javaParam
	varargs := false
	for i < end {
		for p.tok(i).is("@") || p.tok(i).ident("final") {
			if p.tok(i).is("@") {
				i = p.skipAnnotation(i)
			} else {
				i++
			}
		}
		typ, next, ok := p.parseType(i)
		if !ok {
			return out, varargs
		}
		if p.tok(next).is("...") {
			typ.dims++
			varargs = true
			next++
		}
		param := javaParam{typ: typ}
		if p.tok(next).kind == tokIdent {
			param.name = p.tok(next).text
			next++
		}
		for p.tok(next).is("[") && p.tok(next+1).is("]") {
			param.typ.dims++
			next += 2
		}
		out = append(out, param)
		i = next
		if p.tok(i).is(",") {
			i++
		} else {
			break
		}
	}
	return out, varargs
}

// typeDecl parses a class, interface, enum, record or annotation type whose
// keyword is at i.
func (p *javaParser) typeDecl(i int, outer *javaType) (int, error) {
	kind := p.tok(i).text
	t := &javaType{name: p.tok(i + 1).text, outer: outer, file: p.f, enum: kind == "enum", fields: map[string]typeRef{}}
	p.f.types = append(p.f.types, t)
	i += 2
	if p.tok(i).is("<") {
		next := p.skipAngles(i)
		if next < 0 {
			return 0, fmt.Errorf("line %d: bad type parameters on %s", p.tok(i).line, t.name)
		}
		t.typeParams = p.typeParams(i, next)
		i = next
	}
	if p.tok(i).is("(") { // record components
		params, _ := p.params(i+1, p.f.match[i])
		for _, c := range params {
			t.fields[c.name] = c.typ
			t.methods = append(t.methods, &javaMethod{name: c.name, ret: c.typ})
		}
		i = p.f.match[i] + 1
	}
	for !p.tok(i).is("{") {
		if i >= len(p.f.toks) {
			return 0, fmt.Errorf("type %s has no body", t.name)
		}
		clause := p.tok(i).text
		if clause != "extends" && clause != "implements" && clause != "permits" {
			i++
			continue
		}
		i++
		for {
			ref, next, ok := p.parseType(i)
			if !ok {
				break
			}
			switch {
			case clause == "extends" && kind == "class":
				t.extends = ref
			case clause != "permits":
				t.implements = append(t.implements, ref)
			}
			i = next
			if !p.tok(i).is(",") {
				break
			}
			i++
		}
	}
	end := p.f.match[i]
	i++
	if t.enum {
		for i < end && !p.tok(i).is(";") {
			switch tk := p.tok(i); {
			case tk.is("@"):
				i = p.skipAnnotation(i)
				continue
			case tk.kind == tokIdent:
				t.fields[tk.text] = typeRef{name: t.name}
			case tk.is("(") || tk.is("{"):
				i = p.f.match[i]
			}
			i++
		}
		if i < end {
			i++
		}
	}
	for i < end {
		next, err := p.member(i, t)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", t.name, err)
		}
		i = next
	}
	return end + 1, nil
}

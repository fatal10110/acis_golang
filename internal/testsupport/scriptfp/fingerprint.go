// Package scriptfp fingerprints the reference server's script classes and
// the Go script packages that port them, so a test can require that a port
// keeps the reference script's distinctive literals, its engine calls and
// the parent-call shape of its hooks.
//
// The reference fingerprints are committed (reference.golden), with the
// census of every engine call the reference scripts make (census.golden).
// Regenerate both from an unmodified reference checkout with
//
//	go run ./cmd/scriptfp -java <aCis_gameserver>/java -o internal/testsupport/scriptfp
//
// A fingerprint is static: it is read from source text, not from running
// code. See README.md for the record format and the matching rules.
package scriptfp

import (
	"bufio"
	"bytes"
	_ "embed" // reference.golden
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Fingerprint is the static fingerprint of one reference script class.
type Fingerprint struct {
	// Class is the script key: the source path below ScriptingDir with
	// '.' separators and no extension, as scripts.xml names it.
	Class string
	// Extends is the parent class: a script key for a parent in the
	// script trees, else its simple name. Empty when the class extends
	// nothing.
	Extends string
	// Numbers, Strings and Chars are the distinctive literals, each
	// sorted and unique. Numbers are canonical decimals without sign;
	// 0 and 1 are left out, as is the empty string.
	Numbers []string
	Strings []string
	Chars   []string
	// Calls counts the call sites of each engine call, keyed by its
	// census row ("Owner.method", "Owner.new" for a constructor,
	// "?.method" for a call whose target was not found).
	Calls map[string]int
	// Hooks lists the class's hooks with their parent-call shape, sorted.
	Hooks []Hook
}

// Hook is one overridden on* method of a script class and how it reaches
// the parent's implementation.
type Hook struct {
	Name string
	// Shape is "none", "direct <args>" or "helper <args>", where <args>
	// lists "same" or "changed" per parent call, comma-separated: same
	// when the call passes the hook's parameters unchanged and in order.
	// helper means the hook's own body has no parent call and calls a
	// method of its class that makes one.
	Shape string
}

func (h Hook) String() string { return h.Name + " " + h.Shape }

// Build fingerprints every class file of the reference script trees under
// javaRoot (the reference server's java directory), sorted by key.
func Build(javaRoot string) ([]Fingerprint, error) {
	tr, err := loadTree(javaRoot)
	if err != nil {
		return nil, err
	}
	var out []Fingerprint
	for _, f := range tr.files {
		if f.key == "" {
			continue
		}
		fp, err := tr.fingerprint(f)
		if err != nil {
			return nil, fmt.Errorf("scriptfp: %s: %w", f.key, err)
		}
		out = append(out, fp)
	}
	slices.SortFunc(out, func(a, b Fingerprint) int { return strings.Compare(a.Class, b.Class) })
	return out, nil
}

func (tr *tree) fingerprint(f *javaFile) (Fingerprint, error) {
	if len(f.types) == 0 || f.types[0].outer != nil {
		return Fingerprint{}, fmt.Errorf("no top-level type")
	}
	top := f.types[0]
	fp := Fingerprint{Class: f.key, Calls: map[string]int{}}
	if parent := tr.parent(top); parent != nil && parent.file.key != "" {
		fp.Extends = parent.file.key
	} else {
		fp.Extends = top.extends.name
	}

	numbers, strs, chars := map[string]bool{}, map[string]bool{}, map[string]bool{}
	p := &javaParser{f: f}
	for i := 0; i < len(f.toks); i++ {
		t := f.toks[i]
		switch {
		case t.is("@") && !p.tok(i+1).ident("interface"):
			i = p.skipAnnotation(i) - 1
		case t.kind == tokInt || t.kind == tokFloat:
			if t.text != "0" && t.text != "1" {
				numbers[t.text] = true
			}
		case t.kind == tokString && t.text != "":
			strs[t.text] = true
		case t.kind == tokChar:
			chars[t.text] = true
		}
	}
	fp.Numbers = sortNumbers(slices.Collect(maps.Keys(numbers)))
	fp.Strings = slices.Sorted(maps.Keys(strs))
	fp.Chars = slices.Sorted(maps.Keys(chars))

	open := closers(f)
	for _, t := range f.types {
		for _, m := range t.methods {
			if m.body[1] <= m.body[0] {
				continue
			}
			for _, c := range newBody(tr, f, t, m, open).sites() {
				if c.kind == callEngine || c.kind == callUnresolved {
					fp.Calls[c.row()]++
				}
			}
		}
	}

	parent := tr.parent(top)
	for _, m := range top.methods {
		if m.pseudo || m.ctor || m.static || m.private || !m.hasBody {
			continue
		}
		if parent == nil {
			continue
		}
		if _, base := tr.findMethod(parent, m.name, -1); base == nil {
			continue
		}
		b := newBody(tr, f, top, m, open)
		shape := "none"
		if direct := b.superCalls(m.name); len(direct) > 0 {
			shape = "direct " + argShape(direct)
		} else {
			for _, c := range b.sites() {
				if c.kind != callOwn || c.owner != top {
					continue
				}
				_, helper := tr.findMethod(top, c.name, -1)
				if helper == nil || helper == m || !helper.hasBody {
					continue
				}
				// The helper passes its own parameters on; "same" there means
				// unchanged relative to the helper.
				if via := newBody(tr, f, top, helper, open).superCalls(m.name); len(via) > 0 {
					shape = "helper " + argShape(via)
					break
				}
			}
		}
		fp.Hooks = append(fp.Hooks, Hook{Name: m.name, Shape: shape})
	}
	slices.SortStableFunc(fp.Hooks, func(a, b Hook) int { return strings.Compare(a.Name, b.Name) })
	return fp, nil
}

// isHookName reports whether name has the on<Upper> form of a hook.
func isHookName(name string) bool {
	return len(name) > 2 && strings.HasPrefix(name, "on") && name[2] >= 'A' && name[2] <= 'Z'
}

func argShape(same []bool) string {
	parts := make([]string, len(same))
	for i, s := range same {
		parts[i] = "changed"
		if s {
			parts[i] = "same"
		}
	}
	return strings.Join(parts, ",")
}

func sortNumbers(nums []string) []string {
	slices.SortFunc(nums, func(a, b string) int {
		fa, _ := strconv.ParseFloat(a, 64)
		fb, _ := strconv.ParseFloat(b, 64)
		if fa != fb {
			if fa < fb {
				return -1
			}
			return 1
		}
		return strings.Compare(a, b)
	})
	return nums
}

const referenceHeader = "# Static fingerprints of the reference script classes: generated by cmd/scriptfp, do not edit.\n" +
	"# Format: README.md in this directory.\n"

// Render writes fingerprints in the committed line format.
func Render(fps []Fingerprint) []byte {
	var b bytes.Buffer
	b.WriteString(referenceHeader)
	fmt.Fprintf(&b, "classes %d\n", len(fps))
	for _, fp := range fps {
		b.WriteString("\nclass " + fp.Class)
		if fp.Extends != "" {
			b.WriteString(" extends " + fp.Extends)
		}
		b.WriteByte('\n')
		if len(fp.Numbers) > 0 {
			b.WriteString("  numbers " + strings.Join(fp.Numbers, " ") + "\n")
		}
		for _, s := range fp.Strings {
			b.WriteString("  string " + strconv.Quote(s) + "\n")
		}
		for _, c := range fp.Chars {
			b.WriteString("  char " + strconv.Quote(c) + "\n")
		}
		for _, row := range slices.Sorted(maps.Keys(fp.Calls)) {
			fmt.Fprintf(&b, "  call %s %d\n", row, fp.Calls[row])
		}
		for _, h := range fp.Hooks {
			b.WriteString("  hook " + h.String() + "\n")
		}
	}
	return b.Bytes()
}

// Parse reads fingerprints written by Render.
func Parse(data []byte) ([]Fingerprint, error) {
	var out []Fingerprint
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "classes ") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "class "); ok {
			fp := Fingerprint{Calls: map[string]int{}}
			fp.Class, fp.Extends, _ = strings.Cut(rest, " extends ")
			out = append(out, fp)
			continue
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("scriptfp: line %d: record before any class", n)
		}
		fp := &out[len(out)-1]
		field, value, _ := strings.Cut(strings.TrimPrefix(line, "  "), " ")
		switch field {
		case "numbers":
			fp.Numbers = strings.Fields(value)
		case "string", "char":
			s, err := strconv.Unquote(value)
			if err != nil {
				return nil, fmt.Errorf("scriptfp: line %d: %w", n, err)
			}
			if field == "string" {
				fp.Strings = append(fp.Strings, s)
			} else {
				fp.Chars = append(fp.Chars, s)
			}
		case "call":
			row, count, _ := strings.Cut(value, " ")
			sites, err := strconv.Atoi(count)
			if err != nil {
				return nil, fmt.Errorf("scriptfp: line %d: %w", n, err)
			}
			fp.Calls[row] = sites
		case "hook":
			name, shape, _ := strings.Cut(value, " ")
			fp.Hooks = append(fp.Hooks, Hook{Name: name, Shape: shape})
		default:
			return nil, fmt.Errorf("scriptfp: line %d: unknown record %q", n, field)
		}
	}
	return out, sc.Err()
}

//go:embed reference.golden
var committedReference []byte

// Reference returns the committed reference fingerprints, keyed by class.
func Reference() (map[string]Fingerprint, error) {
	fps, err := Parse(committedReference)
	if err != nil {
		return nil, err
	}
	return byClass(fps), nil
}

func byClass(fps []Fingerprint) map[string]Fingerprint {
	out := make(map[string]Fingerprint, len(fps))
	for _, fp := range fps {
		out[fp.Class] = fp
	}
	return out
}

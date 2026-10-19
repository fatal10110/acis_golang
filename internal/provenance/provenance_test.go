package provenance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// skippedDirs are the trees, relative to the module root, whose comments may
// name another implementation: test-only support code, and the fingerprint
// command whose job is to read foreign source files.
var skippedDirs = map[string]bool{
	".git":                    true,
	"tests":                   true,
	"internal/gameservertest": true,
	"internal/testsupport":    true,
	"cmd/scriptfp":            true,
}

var (
	// theReference matches "the reference" and "the reference's" with the
	// word after it; "the reference price" of manor and store data is allowed.
	theReference = regexp.MustCompile(`(?i)\bthe reference(?:'s)?\b(\s+\w+)?`)
	// marker matches the other provenance markers outright.
	marker = regexp.MustCompile(`(?i)\breference's\b|\breference (?:server|source|class|classes|enum|implementation|code)\b|\bJava\b|\b\w+\.java\b`)
)

// foreignName matches an identifier with a "java" word in it: javaInt,
// JavaRound, parseJavaInt.
var foreignName = regexp.MustCompile(`^[jJ]ava(?:[A-Z0-9_]|$)|[a-z0-9_]Java(?:[A-Z0-9_]|$)`)

// markers returns the provenance markers in a comment's text.
func markers(text string) []string {
	text = strings.Join(strings.Fields(text), " ")
	var found []string
	for _, m := range theReference.FindAllStringSubmatch(text, -1) {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(m[1])), "price") {
			found = append(found, m[0])
		}
	}
	return append(found, marker.FindAllString(text, -1)...)
}

func TestMarkers(t *testing.T) {
	for text, want := range map[string]bool{
		"defaulting as the reference does":          true,
		"in the reference's\norder":                 true,
		"The reference\nadds the stones":            true,
		"names the reference class of an NPC":       true,
		"as a Java (int) cast does":                 true,
		"see GeoEngine.java":                        true,
		"each seed's reference price":               false,
		"the reference price the table reports":     false,
		"a #table reference resolves first":         false,
		"an \"id-level\" skill reference":           false,
		"Cast carries no reference to global state": false,
		"javaInt narrows v to an int32":             false,
	} {
		if got := markers(text); (len(got) > 0) != want {
			t.Errorf("markers(%q) = %q, want found = %t", text, got, want)
		}
	}
}

// foreignNames returns the identifiers f declares whose name carries a
// provenance word.
func foreignNames(f *ast.File) []*ast.Ident {
	var found []*ast.Ident
	add := func(ids ...*ast.Ident) {
		for _, id := range ids {
			if id != nil && foreignName.MatchString(id.Name) {
				found = append(found, id)
			}
		}
	}
	addExprs := func(exprs ...ast.Expr) {
		for _, e := range exprs {
			if id, ok := e.(*ast.Ident); ok {
				add(id)
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			add(n.Name)
		case *ast.TypeSpec:
			add(n.Name)
		case *ast.ValueSpec:
			add(n.Names...)
		case *ast.Field:
			add(n.Names...)
		case *ast.ImportSpec:
			add(n.Name)
		case *ast.LabeledStmt:
			add(n.Label)
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				addExprs(n.Lhs...)
			}
		case *ast.RangeStmt:
			if n.Tok == token.DEFINE {
				addExprs(n.Key, n.Value)
			}
		}
		return true
	})
	return found
}

func TestForeignNames(t *testing.T) {
	const src = `package p

import javaFmt "fmt"

const JavaLimit, plain = 1, 2

var parseJavaInt = javaFmt.Sprint

type javaType struct{ JavaField, field int }

func javaFunc(javaArg int) (javaResult int) {
	javaLocal := javaArg
	for javaKey, value := range []int{javaLocal} {
		_ = javaKey + value
	}
javaLabel:
	for {
		break javaLabel
	}
	return javaResult
}

func (javaType) JavaMethod() {}

func javascriptName(java, Javanese, ajava int) { _ = javaFunc(java) }
`
	f, err := parser.ParseFile(token.NewFileSet(), "p.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, id := range foreignNames(f) {
		got = append(got, id.Name)
	}
	want := []string{
		"javaFmt", "JavaLimit", "parseJavaInt", "javaType", "JavaField", "javaFunc", "javaArg",
		"javaResult", "javaLocal", "javaKey", "javaLabel", "JavaMethod", "java",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("foreignNames = %q, want %q", got, want)
	}
}

// Production files carry no provenance markers in their comments and no
// provenance word in the names they declare.
func TestProductionFilesHaveNoProvenanceMarkers(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root %s: %v", root, err)
	}
	fset := token.NewFileSet()
	files := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skippedDirs[rel] || d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		files++
		for _, group := range f.Comments {
			for _, m := range markers(group.Text()) {
				t.Errorf("%s:%d: comment says %q; state the behavior directly", rel, fset.Position(group.Pos()).Line, m)
			}
		}
		for _, id := range foreignNames(f) {
			t.Errorf("%s:%d: identifier %s names another implementation; name what it does", rel, fset.Position(id.Pos()).Line, id.Name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 100 {
		t.Fatalf("checked %d production files under %s, want the whole module", files, root)
	}
}

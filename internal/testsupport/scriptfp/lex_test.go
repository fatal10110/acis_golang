package scriptfp

import (
	"reflect"
	"testing"
)

func TestLexJavaLiteralsAndOperators(t *testing.T) {
	src := `/* block "not a string" */ int a = 30_048; // 777
long b = 0x1F + 0b101 + 017 + 3000L;
double c = 0.5f + .25 + 1e3 + 2.0d + 1.;
String s = "tab\there \"q\" é \101";
char d = '\''; f(x -> x::y, int... z);`
	toks, err := lexJava(src)
	if err != nil {
		t.Fatalf("lexJava: %v", err)
	}
	var lits []string
	var ops []string
	for _, tk := range toks {
		switch tk.kind {
		case tokInt, tokFloat, tokString, tokChar:
			lits = append(lits, tk.text)
		case tokOp:
			if len(tk.text) > 1 {
				ops = append(ops, tk.text)
			}
		}
	}
	wantLits := []string{"30048", "31", "5", "15", "3000", "0.5", "0.25", "1000", "2", "1", "tab\there \"q\" é A", "'"}
	if !reflect.DeepEqual(lits, wantLits) {
		t.Errorf("literals = %q, want %q", lits, wantLits)
	}
	if want := []string{"->", "::", "..."}; !reflect.DeepEqual(ops, want) {
		t.Errorf("long operators = %q, want %q", ops, want)
	}
	if last := toks[len(toks)-1]; last.line != 5 {
		t.Errorf("last token on line %d, want 5", last.line)
	}
}

func TestLexJavaRejectsMalformedInput(t *testing.T) {
	for _, src := range []string{`"open`, `/* open`, `int a = 12abc;`, `'\q'`, `String s = """block""";`} {
		if _, err := lexJava(src); err == nil {
			t.Errorf("lexJava(%q) succeeded, want an error", src)
		}
	}
}

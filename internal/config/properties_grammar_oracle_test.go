package config

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestGettersMatchJavaGrammar pins the typed getters to the reference's
// ExProperties on one value each: the int, long and double scalars, the
// boolean, the delimited string and int lists, and the two item-pair lists.
// Every line of testdata/grammar/cases.txt is the text after "key=" of a
// properties file.
//
// java.golden is the oracle: testdata/grammar/run.sh compiles GrammarProbe.java
// against the reference's own ExProperties and runs it over the cases
// (OpenJDK 21, eclipse-temurin:21-jdk-alpine). The Go side dumps the same
// lines; a getter that fails is "ERROR", and an item-pair list the reference
// reads as empty on a malformed entry is "<>".
func TestGettersMatchJavaGrammar(t *testing.T) {
	// The getters log the malformed values through one process-wide logger.
	captureConfigLog(t)
	dir := filepath.Join("testdata", "grammar")
	raw, err := os.ReadFile(filepath.Join(dir, "cases.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(dir, "java.golden"))
	if err != nil {
		t.Fatal(err)
	}
	wantByCase := splitProbeDump(string(want))
	cases := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	for i, value := range cases {
		name := fmt.Sprintf("%d [%s]", i, value)
		w, ok := wantByCase[name]
		if !ok {
			t.Errorf("case %s: missing from java.golden; rerun testdata/grammar/run.sh", name)
			continue
		}
		delete(wantByCase, name)
		if got := grammarDump(t, value); got != w {
			t.Errorf("case %s:\ngot:\n%swant:\n%s", name, got, w)
		}
	}
	for name := range wantByCase {
		t.Errorf("case %s: in java.golden but not in cases.txt", name)
	}
}

// grammarDump reads value through every getter, in the probe's format.
func grammarDump(t *testing.T, value string) string {
	t.Helper()
	p, err := ParseString("k=" + value + "\n")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	line := func(name string, v string, err error) {
		if err != nil {
			v = "ERROR"
		} else {
			v = "<" + v + ">"
		}
		fmt.Fprintf(&b, "%s=%s\n", name, v)
	}
	n, err := p.Int("k", 0)
	line("int", strconv.Itoa(n), err)
	n64, err := p.Int64("k", 0)
	line("long", strconv.FormatInt(n64, 10), err)
	f, err := p.Float64("k", 0)
	bits := math.Float64bits(f)
	if math.IsNaN(f) {
		bits = 0x7ff8000000000000 // the one NaN doubleToLongBits returns
	}
	line("double", strconv.FormatUint(bits, 16), err)
	line("bool", strconv.FormatBool(p.Bool("k", false)), nil)
	line("strings", strings.Join(p.Strings("k", []string{}), "|"), nil)
	ints, err := p.Ints("k", []int{})
	var list strings.Builder
	for _, v := range ints {
		list.WriteString(strconv.Itoa(v) + "|")
	}
	line("ints", list.String(), err)
	line("pairs", pairList(p.IntPairs("k", "")), nil)
	pairs, err := p.IntPairsComma("k", "")
	line("commapairs", pairList(pairs), err)
	return b.String()
}

func pairList(pairs []IntPair) string {
	var b strings.Builder
	for _, p := range pairs {
		fmt.Fprintf(&b, "%d-%d|", p.First, p.Second)
	}
	return b.String()
}

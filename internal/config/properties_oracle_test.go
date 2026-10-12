package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

// TestParseMatchesJavaProperties pins Parse to java.util.Properties.load on
// an InputStream, the loader behind every reference .properties read. Each
// testdata/properties/cases file is raw file bytes covering separators,
// whitespace, comments (including ones ending in a backslash), LF/CRLF/CR
// continuations of keys and values, empty keys, escapes, \u surrogate
// pairs, malformed \u escapes, ISO-8859-1 bytes and a UTF-8 byte order mark.
//
// java.golden is the oracle: testdata/properties/run.sh compiles
// PropertiesProbe.java and runs it over cases/ (OpenJDK 21.0.12.1,
// eclipse-temurin:21-jdk-alpine). The Go side dumps the same sorted
// "[key]=[value]" form, so a Go string must hold exactly the Java string's
// characters. A lone \u surrogate has no UTF-8 form and is not covered.
func TestParseMatchesJavaProperties(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("testdata", "properties")
	want, err := os.ReadFile(filepath.Join(dir, "java.golden"))
	if err != nil {
		t.Fatal(err)
	}
	names, err := filepath.Glob(filepath.Join(dir, "cases", "*.properties"))
	if err != nil || len(names) == 0 {
		t.Fatalf("no cases: %v", err)
	}
	slices.Sort(names)

	wantByFile := splitProbeDump(string(want))
	for _, name := range names {
		base := filepath.Base(name)
		var got strings.Builder
		if p, err := LoadFile(name); err != nil {
			got.WriteString("ERROR\n")
		} else {
			keys := p.Keys()
			slices.SortFunc(keys, compareUTF16)
			for _, key := range keys {
				value, _ := p.Lookup(key)
				fmt.Fprintf(&got, "[%s]=[%s]\n", probeEscape(key), probeEscape(value))
			}
		}
		w, ok := wantByFile[base]
		if !ok {
			t.Errorf("%s: missing from java.golden; rerun testdata/properties/run.sh", base)
			continue
		}
		delete(wantByFile, base)
		if got.String() != w {
			t.Errorf("%s:\ngot:\n%swant:\n%s", base, got.String(), w)
		}
	}
	for base := range wantByFile {
		t.Errorf("%s: in java.golden but not in cases/", base)
	}
}

// splitProbeDump maps each "== name" section of a probe dump to its lines.
func splitProbeDump(dump string) map[string]string {
	out := make(map[string]string)
	name := ""
	for line := range strings.Lines(dump) {
		if rest, ok := strings.CutPrefix(line, "== "); ok {
			name = strings.TrimSuffix(rest, "\n")
			out[name] = ""
			continue
		}
		out[name] += line
	}
	return out
}

// probeEscape renders s the way PropertiesProbe.java does: printable ASCII
// as is with a backslash doubled, every other UTF-16 unit as \uXXXX. A byte
// that is not valid UTF-8 renders as \xNN, which the probe never prints.
func probeEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, `\x%02X`, s[i])
			i++
			continue
		}
		i += size
		for _, u := range utf16.Encode([]rune{r}) {
			switch {
			case u == '\\':
				b.WriteString(`\\`)
			case u >= 0x20 && u < 0x7f:
				b.WriteByte(byte(u))
			default:
				fmt.Fprintf(&b, `\u%04X`, u)
			}
		}
	}
	return b.String()
}

// compareUTF16 orders strings as Java's String.compareTo does.
func compareUTF16(a, b string) int {
	return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b)))
}

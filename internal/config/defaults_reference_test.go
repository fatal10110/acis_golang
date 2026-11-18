package config

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var updateDefaults = flag.Bool("update-defaults", false, "rewrite testdata/reference_defaults.golden from the reference loader")

// referenceDefault is one key's default as the reference loader's
// getProperty call spells it.
type referenceDefault struct {
	file, key string
	// kind is bool, int, long, double, string, strings, ints or none.
	kind string
	// value is the literal as written, with a list's elements joined by ",".
	value string
}

func (d referenceDefault) line() string {
	return d.file + " " + d.key + " " + d.kind + " " + strconv.Quote(d.value)
}

var (
	// referenceScalarDefault matches getProperty("Key", <literal>): a bool,
	// an int, a long (L suffix), a double or a string literal.
	referenceScalarDefault = regexp.MustCompile(`\b(\w+)\.getProperty\("([^"]+)",\s*(true|false|-?\d+\.\d*|-?\.\d+|-?\d+L?|"[^"]*")\)`)
	// referenceListDefault matches a String[] or int[] array default.
	referenceListDefault = regexp.MustCompile(`(?s)\b(\w+)\.getProperty\("([^"]+)",\s*new (String|int)\[\]\s*\{([^}]*)\}`)
	// referenceNullDefault matches the one list read whose default is null.
	referenceNullDefault = regexp.MustCompile(`\b(\w+)\.getProperty\("([^"]+)",\s*\(String\[\]\) null`)
	// referenceBareRead matches a read with no default.
	referenceBareRead = regexp.MustCompile(`\b(\w+)\.getProperty\("([^"]+)"\)`)
)

// referenceDefaults parses every getProperty default of the reference
// loader, by the properties file the variable it is read from holds.
func referenceDefaults(t *testing.T, src string) []referenceDefault {
	t.Helper()
	files := map[string]string{}
	for _, m := range referenceFileConst.FindAllStringSubmatch(src, -1) {
		files[m[1]] = m[2]
	}
	loads := referenceLoad.FindAllStringSubmatchIndex(src, -1)
	var out []referenceDefault
	for i, m := range loads {
		end := len(src)
		if i+1 < len(loads) {
			end = loads[i+1][0]
		}
		variable, constant := src[m[2]:m[3]], src[m[4]:m[5]]
		file, ok := files[constant]
		if !ok {
			t.Fatalf("initProperties(%s): no such file constant", constant)
		}
		if !strings.HasSuffix(file, ".properties") {
			continue // hexid.txt is the registration, no settings file
		}
		body := src[m[1]:end]
		for _, d := range referenceDefaultsIn(body, variable) {
			d.file = file
			out = append(out, d)
		}
	}
	slices.SortFunc(out, func(a, b referenceDefault) int {
		return strings.Compare(a.file+" "+a.key, b.file+" "+b.key)
	})
	return out
}

func referenceDefaultsIn(body, variable string) []referenceDefault {
	var out []referenceDefault
	for _, m := range referenceScalarDefault.FindAllStringSubmatch(body, -1) {
		if m[1] != variable {
			continue
		}
		lit := m[3]
		d := referenceDefault{key: m[2], value: lit}
		switch {
		case lit == "true" || lit == "false":
			d.kind = "bool"
		case strings.HasPrefix(lit, `"`):
			d.kind, d.value = "string", lit[1:len(lit)-1]
		case strings.HasSuffix(lit, "L"):
			d.kind, d.value = "long", strings.TrimSuffix(lit, "L")
		case strings.Contains(lit, "."):
			d.kind = "double"
		default:
			d.kind = "int"
		}
		out = append(out, d)
	}
	for _, m := range referenceListDefault.FindAllStringSubmatch(body, -1) {
		if m[1] != variable {
			continue
		}
		var elems []string
		for _, e := range strings.Split(m[4], ",") {
			if e = strings.Trim(strings.TrimSpace(e), `"`); e != "" {
				elems = append(elems, e)
			}
		}
		kind := "strings"
		if m[3] == "int" {
			kind = "ints"
		}
		out = append(out, referenceDefault{key: m[2], kind: kind, value: strings.Join(elems, ",")})
	}
	for _, m := range referenceNullDefault.FindAllStringSubmatch(body, -1) {
		if m[1] == variable {
			out = append(out, referenceDefault{key: m[2], kind: "none"})
		}
	}
	for _, m := range referenceBareRead.FindAllStringSubmatch(body, -1) {
		if m[1] == variable {
			out = append(out, referenceDefault{key: m[2], kind: "none"})
		}
	}
	return out
}

const defaultsGolden = "reference_defaults.golden"

// TestReferenceDefaultsGolden rebuilds testdata/reference_defaults.golden
// from the reference loader: every key's file, kind and default. It needs the
// reference source beside the module and skips without it, or fails as
// datapack.Reference decides. Run with -update-defaults to rewrite the golden.
func TestReferenceDefaultsGolden(t *testing.T) {
	src := readConfigSource(t)
	var want []string
	for _, d := range referenceDefaults(t, src) {
		want = append(want, d.line())
	}
	path := filepath.Join("testdata", defaultsGolden)
	if *updateDefaults {
		if err := os.WriteFile(path, []byte("# <file> <key> <kind> <default as getProperty spells it>\n"+strings.Join(want, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			got = append(got, line)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("%s is stale; run with -update-defaults (reference has %d entries, golden %d)", defaultsGolden, len(want), len(got))
	}
}

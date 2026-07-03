package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseProperties(t *testing.T) {
	p, err := ParseString(`
# comment
plain = value
space separator
colon:value
escaped\ key = escaped\ value
unicode = \u0041
continued = one,\
  two
duplicate = first
duplicate = second
empty
`)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]string{
		"plain":       "value",
		"space":       "separator",
		"colon":       "value",
		"escaped key": "escaped value",
		"unicode":     "A",
		"continued":   "one,two",
		"duplicate":   "second",
		"empty":       "",
	}
	for key, want := range tests {
		got, ok := p.Lookup(key)
		if !ok {
			t.Fatalf("Lookup(%q): missing", key)
		}
		if got != want {
			t.Fatalf("Lookup(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestTypedGetters(t *testing.T) {
	p, err := ParseString(`
truth = True
notTruth = yes
int = 42
int64 = 9000000000
float = 1.5
words = a, b; c
ints = 1,2;3
pairs = 57-100;6651-3
badInt = nope
`)
	if err != nil {
		t.Fatal(err)
	}

	if got := p.Bool("truth", false); !got {
		t.Fatal("Bool truth = false, want true")
	}
	if got := p.Bool("notTruth", true); got {
		t.Fatal("Bool notTruth = true, want false")
	}
	if got := p.Bool("missingBool", true); !got {
		t.Fatal("Bool missingBool = false, want default true")
	}
	if got, err := p.Int("int", 0); err != nil || got != 42 {
		t.Fatalf("Int = %d, %v; want 42, nil", got, err)
	}
	if got, err := p.Int64("int64", 0); err != nil || got != 9000000000 {
		t.Fatalf("Int64 = %d, %v; want 9000000000, nil", got, err)
	}
	if got, err := p.Float64("float", 0); err != nil || got != 1.5 {
		t.Fatalf("Float64 = %f, %v; want 1.5, nil", got, err)
	}
	if got := p.Strings("words", nil); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("Strings = %#v", got)
	}
	if got, err := p.Ints("ints", nil); err != nil || !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("Ints = %#v, %v", got, err)
	}
	if got, err := p.IntPairs("pairs", ""); err != nil || !reflect.DeepEqual(got, []IntPair{{57, 100}, {6651, 3}}) {
		t.Fatalf("IntPairs = %#v, %v", got, err)
	}
	if got, err := p.Int("missingInt", 7); err != nil || got != 7 {
		t.Fatalf("missing Int = %d, %v; want 7, nil", got, err)
	}
	if _, err := p.Int("badInt", 0); err == nil {
		t.Fatal("bad Int: expected error")
	}
}

func TestLoadDirectoryAndAudit(t *testing.T) {
	dir := t.TempDir()
	for _, name := range ShippedFiles {
		body := ""
		if name == "server.properties" {
			body = "Known = 1\nUnknown = 2\n"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	set, err := LoadDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(set.Files); got != len(ShippedFiles) {
		t.Fatalf("loaded %d files, want %d", got, len(ShippedFiles))
	}
	if got := set.CountMismatches(map[string]int{"server.properties": 2, "banned_ips.properties": 0}); len(got) != 0 {
		t.Fatalf("CountMismatches = %#v", got)
	}
	if got := set.UnknownKeys(map[string][]string{"server.properties": {"Known"}}); !reflect.DeepEqual(got, []KeyRef{{File: "server.properties", Key: "Unknown"}}) {
		t.Fatalf("UnknownKeys = %#v", got)
	}

	if err := os.Remove(filepath.Join(dir, "siege.properties")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDirectory(dir); err == nil {
		t.Fatal("LoadDirectory with missing shipped file: expected error")
	}
}

func TestLoadConfigDirectoryFromEnvironment(t *testing.T) {
	dir := os.Getenv("ACIS_CONFIG_DIR")
	if dir == "" {
		t.Skip("set ACIS_CONFIG_DIR to smoke-test real config files")
	}

	set, err := LoadDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(set.Files); got != len(ShippedFiles) {
		t.Fatalf("loaded %d files, want %d", got, len(ShippedFiles))
	}
	if got := set.Files["geoengine.properties"].String("GeoDataType", ""); got != "L2OFF" {
		t.Fatalf("GeoDataType = %q, want L2OFF", got)
	}
	if _, ok := set.Files["geoengine.properties"].Lookup("16_10"); !ok {
		t.Fatal("geoengine region key 16_10 missing")
	}
	got := set.Files["players.properties"].Strings("ListOfPetItems", nil)
	if len(got) < 3 || !reflect.DeepEqual(got[:3], []string{"2375", "3500", "3501"}) {
		t.Fatalf("ListOfPetItems = %#v", got)
	}
}

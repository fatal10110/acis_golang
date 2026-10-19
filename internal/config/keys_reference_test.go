package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// configSource is the reference configuration loader, relative to the
// workspace that holds the reference checkout.
const configSource = "aCis_gameserver/java/net/sf/l2j/Config.java"

var (
	// referenceFileConst matches a properties file constant:
	// NAME_FILE = "./config/<file>".
	referenceFileConst = regexp.MustCompile(`(\w+_FILE) = "\./config/([^"]+)"`)
	// referenceLoad matches the start of one file's reads:
	// final ExProperties <var> = initProperties(<NAME_FILE>);
	referenceLoad = regexp.MustCompile(`final ExProperties (\w+) = initProperties\((?:Config\.)?(\w+)\);`)
)

// TestReferenceKeysGolden rebuilds testdata/reference_keys.golden from the
// reference loader: every key each file's properties are read by, up to
// the next file's. It needs the reference source beside the module and
// skips without it, or fails when ACIS_REQUIRE_DATAPACK is set.
func TestReferenceKeysGolden(t *testing.T) {
	t.Parallel()
	src := readConfigSource(t)
	files := map[string]string{}
	for _, m := range referenceFileConst.FindAllStringSubmatch(src, -1) {
		files[m[1]] = m[2]
	}
	loads := referenceLoad.FindAllStringSubmatchIndex(src, -1)
	keys := map[string]bool{}
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
		read := regexp.MustCompile(`\b` + variable + `\.\w+\("([^"]+)"`)
		for _, k := range read.FindAllStringSubmatch(src[m[1]:end], -1) {
			keys[file+" "+k[1]] = true
		}
	}
	var want []string
	for line := range keys {
		want = append(want, line)
	}
	slices.Sort(want)
	var got []string
	for file, ks := range referenceKeys(t) {
		for _, k := range ks {
			got = append(got, file+" "+k)
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("reference_keys.golden is stale; the reference reads:\n%s", strings.Join(want, "\n"))
	}
}

// readConfigSource returns the reference loader's source, found above the
// module root directly or under an acis_public directory.
func readConfigSource(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		for _, candidate := range []string{filepath.Join(dir, configSource), filepath.Join(dir, "acis_public", configSource)} {
			if b, err := os.ReadFile(candidate); err == nil {
				return string(b)
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	msg := fmt.Sprintf("%s not found above the module root", configSource)
	if os.Getenv("ACIS_REQUIRE_DATAPACK") != "" {
		t.Fatal(msg + " and ACIS_REQUIRE_DATAPACK is set")
	}
	t.Skip(msg + " (set ACIS_REQUIRE_DATAPACK=1 to fail instead)")
	return ""
}

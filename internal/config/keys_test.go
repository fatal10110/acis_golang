package config

import (
	"bufio"
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// pendingKeys are reference keys no reader takes yet. Each belongs to the
// open issue that ports its consumer; the boot reports them as unsupported.
var pendingKeys = map[string][]string{
	"events.properties": {
		// Dimensional Rift (Q635) and Q620: #184.
		"AnakazelPortChance", "AutoJumpsDelayMin", "AutoJumpsDelayRnd", "CaptainCost", "CommanderCost", "HeroCost", "NeededPartyMembers",
		"OfficerCost", "RecruitCost", "RiftMinPartySize", "SoldierCost",
	},
}

// referenceKeys reads testdata/reference_keys.golden: the keys the
// reference configuration loader reads, by file.
func referenceKeys(t *testing.T) map[string][]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "reference_keys.golden"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		file, key, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("golden line %q is not \"<file> <key>\"", line)
		}
		out[file] = append(out[file], key)
	}
	return out
}

// tableOf returns the key set of file and the server that reads it.
func tableOf(file string) (keySet, bool) {
	if set, ok := loginServerKeys[file]; ok {
		return set, true
	}
	set, ok := gameServerKeys[file]
	return set, ok
}

// TestKeyTablesCoverTheReferenceKeys pins the key tables against every key
// the reference loader reads: each is read, ignored or pending, exactly
// once, and the tables hold no other key.
func TestKeyTablesCoverTheReferenceKeys(t *testing.T) {
	t.Parallel()
	ref := referenceKeys(t)
	files := len(gameServerKeys) + len(loginServerKeys)
	if len(ref) != files {
		t.Fatalf("reference files %d, key tables %d", len(ref), files)
	}
	for file, keys := range ref {
		set, ok := tableOf(file)
		if !ok {
			t.Errorf("%s: no key table", file)
			continue
		}
		var got []string
		got = append(got, set.Read...)
		got = append(got, set.Ignored...)
		got = append(got, pendingKeys[file]...)
		slices.Sort(got)
		if dup := slices.Compact(slices.Clone(got)); len(dup) != len(got) {
			t.Errorf("%s: a key is listed twice among read, ignored and pending", file)
		}
		want := slices.Sorted(slices.Values(keys))
		for _, key := range want {
			if _, found := slices.BinarySearch(got, key); !found {
				t.Errorf("%s %s: read by the reference, but neither read, ignored nor pending here", file, key)
			}
		}
		for _, key := range got {
			if _, found := slices.BinarySearch(want, key); !found {
				t.Errorf("%s %s: in the key tables, but the reference reads no such key", file, key)
			}
		}
	}
}

// TestReadKeysHaveAReader holds the key tables to the code: every Read key
// is spelled as a string literal somewhere in the servers' non-test code,
// and no ignored or pending key is, so wiring one fails here until it moves
// into Read.
func TestReadKeysHaveAReader(t *testing.T) {
	t.Parallel()
	literals := sourceLiterals(t, filepath.Join("..", "..", "cmd"), filepath.Join("..", "..", "internal"))
	for _, tables := range []map[string]keySet{gameServerKeys, loginServerKeys} {
		for file, set := range tables {
			for _, key := range set.Read {
				if !literals[key] {
					t.Errorf("%s %s: listed as read, but no code spells the key", file, key)
				}
			}
			for _, key := range append(slices.Clone(set.Ignored), pendingKeys[file]...) {
				if literals[key] {
					t.Errorf("%s %s: listed as ignored or pending, but code spells the key; move it into Read", file, key)
				}
			}
		}
	}
}

// sourceLiterals returns every string literal of the non-test Go files
// under roots, keys.go itself left out.
func sourceLiterals(t *testing.T, roots ...string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	fset := token.NewFileSet()
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			if filepath.Clean(path) == filepath.Join("..", "..", "internal", "config", "keys.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s, err := strconv.Unquote(lit.Value); err == nil {
						out[s] = true
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// TestUnreadKeysSortsTheKeysNoReaderTakes pins the boot report: a read key
// and a geodata region pass, an ignored key is reported as having no
// effect, any other key as unsupported, each once.
func TestUnreadKeysSortsTheKeysNoReaderTakes(t *testing.T) {
	t.Parallel()
	server, err := ParseString("GameserverPort = 7777\nDeveloper = True\nSendMailTime = 5\nTypoKey = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	unsupported, ignored := UnreadKeys(GameServer, "server.properties", server)
	if !slices.Equal(unsupported, []string{"TypoKey"}) || !slices.Equal(ignored, []string{"Developer", "SendMailTime"}) {
		t.Fatalf("UnreadKeys(server) = %q, %q; want the typo, then Developer and SendMailTime", unsupported, ignored)
	}

	geo, err := ParseString("GeoDataPath = ./data/geodata/\n22_22\n16_10 =\nGeoTypo = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	if unsupported, ignored := UnreadKeys(GameServer, "geoengine.properties", geo); !slices.Equal(unsupported, []string{"GeoTypo"}) || len(ignored) != 0 {
		t.Fatalf("UnreadKeys(geoengine) = %q, %q; want only GeoTypo", unsupported, ignored)
	}

	login, err := ParseString("LoginserverPort = 2106\nDeveloper = True\n")
	if err != nil {
		t.Fatal(err)
	}
	if unsupported, ignored := UnreadKeys(LoginServer, "loginserver.properties", login); !slices.Equal(unsupported, []string{"Developer"}) || len(ignored) != 0 {
		t.Fatalf("UnreadKeys(loginserver) = %q, %q; want Developer unsupported there", unsupported, ignored)
	}

	var out bytes.Buffer
	ReportUnreadKeys(zerolog.New(&out), GameServer, "server.properties", server)
	want := strings.Join([]string{
		`{"level":"warn","file":"server.properties","key":"TypoKey","message":"config key not supported; its value is ignored"}`,
		`{"level":"info","file":"server.properties","key":"Developer","message":"config key has no effect on this server"}`,
		`{"level":"info","file":"server.properties","key":"SendMailTime","message":"config key has no effect on this server"}`,
	}, "\n") + "\n"
	if out.String() != want {
		t.Fatalf("report =\n%s\nwant\n%s", out.String(), want)
	}
}

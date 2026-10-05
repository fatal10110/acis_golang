package scriptpages

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestCommittedIndexMatchesDatapack rebuilds the index from the datapack and
// requires it to equal the committed copy byte for byte, and to hold one
// record per page file under the script tree.
func TestCommittedIndexMatchesDatapack(t *testing.T) {
	dp := datapack.Require(t)

	built, err := Build(dp)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	again, err := Build(dp)
	if err != nil {
		t.Fatalf("Build again: %v", err)
	}
	if !bytes.Equal(built, again) {
		t.Fatal("two builds from the same datapack differ")
	}
	if !bytes.Equal(built, committed) {
		t.Fatal("index.jsonl is stale: regenerate it with go run ./cmd/pageindex -datapack <aCis_datapack> -o internal/testsupport/scriptpages/index.jsonl")
	}

	idx, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var files []string
	root := filepath.Join(dp, filepath.FromSlash(Root))
	if err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		lower := strings.ToLower(d.Name())
		if d.Type().IsRegular() && (strings.HasSuffix(lower, ".htm") || strings.HasSuffix(lower, ".html")) {
			rel, _ := filepath.Rel(root, name)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(files) != len(idx) {
		t.Fatalf("index holds %d pages, the script tree %d", len(idx), len(files))
	}
	for _, name := range files {
		if _, ok := idx[name]; !ok {
			t.Errorf("page %s missing from the index", name)
		}
	}
	for _, dir := range []string{"quest/", "ai/", "feature/", "teleport/", "siegablehall/"} {
		found := false
		for name := range idx {
			if strings.HasPrefix(name, dir) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no page indexed under %s", dir)
		}
	}
}

func TestBuildRecordsBypassesPlaceholdersAndHash(t *testing.T) {
	dp := t.TempDir()
	write := func(name string, data string) {
		t.Helper()
		full := filepath.Join(dp, filepath.FromSlash(Root), filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("quest/Q1/a.htm", "<html><body>\r\n"+
		`<a action="bypass -h Quest Q1 go">go</a>`+"\r\n"+
		`<a action="bypass  npc_%objectId%_Quest ">quest</a>`+"\r"+
		`<edit var="v"><a action="bypass -h Quest Q1 buy $v">buy</a>`+"\n"+
		"%name% has 50% of %objectId%, %name%</body></html>")
	write("quest/Q1/B.HTM", `<a action="bypass x">x</a><a action="bypass -h">cut</a><a action="bypass y">y</a>`)
	write("quest/Q1/bad.html", "caf\xe9")
	write("quest/Q1/notes.txt", "not a page")
	write("ai/A/empty.htm", "")

	data, err := Build(dp)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	idx, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	sum := func(s string) string {
		h := sha256.Sum256([]byte(s))
		return hex.EncodeToString(h[:8])
	}
	want := Index{
		"ai/A/empty.htm": {Name: "ai/A/empty.htm", Hash: sum("")},
		"quest/Q1/a.htm": {
			Name: "quest/Q1/a.htm",
			Hash: sum("<html><body>\n" +
				`<a action="bypass -h Quest Q1 go">go</a>` + "\n" +
				`<a action="bypass  npc_%objectId%_Quest ">quest</a>` + "\n" +
				`<edit var="v"><a action="bypass -h Quest Q1 buy $v">buy</a>` + "\n" +
				"%name% has 50% of %objectId%, %name%</body></html>\n"),
			Bypass:       []string{"Quest Q1 go", "npc_%objectId%_Quest"},
			BypassPrefix: []string{"Quest Q1 buy"},
			Placeholders: []string{"%name%", "%objectId%"},
		},
		// The second link's command span is cut short; the server stops
		// reading links there, so "y" is never admitted.
		"quest/Q1/B.HTM": {
			Name:   "quest/Q1/B.HTM",
			Hash:   sum(`<a action="bypass x">x</a><a action="bypass -h">cut</a><a action="bypass y">y</a>` + "\n"),
			Bypass: []string{"x"},
		},
		"quest/Q1/bad.html": {Name: "quest/Q1/bad.html", Hash: sum("caf\xe9"), InvalidUTF8: true},
	}
	if !reflect.DeepEqual(idx, want) {
		t.Fatalf("index:\n got %+v\nwant %+v", idx, want)
	}
	if bytes.Contains(data, []byte("<html>")) || bytes.Contains(data, []byte("has 50%")) {
		t.Fatalf("index carries page text:\n%s", data)
	}
	if !bytes.HasPrefix(data, []byte(`{"page":"ai/A/empty.htm",`)) {
		t.Fatalf("records are not sorted by page name:\n%s", data)
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	if _, err := Parse([]byte(`{"page":"quest/Q1/a.htm","hash":"00","text":"<html>"}` + "\n")); err == nil {
		t.Fatal("a record with a text field was accepted")
	}
}

func TestCheckPackage(t *testing.T) {
	idx, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	const q001 = "quest/Q001_LettersOfLove"
	tests := []struct {
		name   string
		dir    string
		exempt []string
		want   []string
	}{
		{name: "every page indexed", dir: "present"},
		{
			name: "unknown page names fail",
			dir:  "missing",
			want: []string{
				`missing.go:6: page "30048-99.htm" is not in the script page index (looked up quest/Q001_LettersOfLove/30048-99.htm)`,
				`missing.go:7: page "data/html/script/quest/Q001_LettersOfLove/30033-03.html" is not in the script page index (looked up quest/Q001_LettersOfLove/30033-03.html)`,
			},
		},
		{
			name:   "exempted names pass",
			dir:    "missing",
			exempt: []string{"30048-99.htm", "data/html/script/quest/Q001_LettersOfLove/30033-03.html"},
		},
		{
			name:   "stale exemptions fail",
			dir:    "present",
			exempt: []string{"30048-01.htm", "30048-77.htm"},
			want: []string{
				`exemption "30048-01.htm" names indexed page quest/Q001_LettersOfLove/30048-01.htm`,
				`exemption "30048-77.htm" is not used by the package`,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CheckPackage(idx, filepath.Join("testdata", tc.dir), q001, tc.exempt...)
			if err != nil {
				t.Fatalf("CheckPackage: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("problems:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

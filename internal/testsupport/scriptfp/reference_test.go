package scriptfp

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// requireReference returns the reference server's java directory, found
// above this module as datapack.Find finds the datapack. A missing checkout
// skips the test, or fails it when ACIS_REQUIRE_DATAPACK is set.
func requireReference(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	for dir := filepath.Dir(thisFile); ; {
		for _, candidate := range []string{
			filepath.Join(dir, "aCis_gameserver", "java"),
			filepath.Join(dir, "acis_public", "aCis_gameserver", "java"),
		} {
			if info, err := os.Stat(filepath.Join(candidate, ScriptingDir)); err == nil && info.IsDir() {
				return candidate
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if os.Getenv("ACIS_REQUIRE_DATAPACK") != "" {
		t.Fatal("aCis_gameserver not found above the module root and ACIS_REQUIRE_DATAPACK is set")
	}
	t.Skip("aCis_gameserver not checked out near the module root, skipping reference comparison (set ACIS_REQUIRE_DATAPACK=1 to fail instead)")
	return ""
}

// TestCommittedGoldensMatchReference rebuilds both goldens from the
// reference source and requires them to equal the committed copies.
func TestCommittedGoldensMatchReference(t *testing.T) {
	javaRoot := requireReference(t)
	fps, err := Build(javaRoot)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	again, err := Build(javaRoot)
	if err != nil {
		t.Fatalf("Build again: %v", err)
	}
	if !bytes.Equal(Render(fps), Render(again)) {
		t.Fatal("two builds from the same reference differ")
	}
	if !bytes.Equal(Render(fps), committedReference) {
		t.Fatal("reference.golden is stale: regenerate it with go run ./cmd/scriptfp (see the package comment)")
	}
	amap, err := APIMap()
	if err != nil {
		t.Fatalf("APIMap: %v", err)
	}
	if !bytes.Equal(Census(fps, amap), committedCensus) {
		t.Fatal("census.golden is stale: regenerate it with go run ./cmd/scriptfp (see the package comment)")
	}
}

// TestEveryListedScriptHasAFingerprint requires a committed fingerprint for
// every scripts.xml entry, so the census covers all listed scripts.
func TestEveryListedScriptHasAFingerprint(t *testing.T) {
	raw, err := os.ReadFile(datapack.Path(t, "data", "xml", "scripts.xml"))
	if err != nil {
		t.Fatalf("read scripts.xml: %v", err)
	}
	var doc struct {
		Scripts []struct {
			Path string `xml:"path,attr"`
		} `xml:"script"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse scripts.xml: %v", err)
	}
	if len(doc.Scripts) == 0 {
		t.Fatal("scripts.xml lists no script")
	}
	ref, err := Reference()
	if err != nil {
		t.Fatalf("Reference: %v", err)
	}
	for _, s := range doc.Scripts {
		if _, ok := ref[s.Path]; !ok {
			t.Errorf("listed script %s has no fingerprint", s.Path)
		}
	}
}

var manifestHook = regexp.MustCompile(`^  hook (\w+)\(`)

var manifestSuper = regexp.MustCompile(`super(?::\w+)?(?:@\S+?)?\(args=(same|changed),`)

// TestHooksMatchProbeManifest checks the static parent-call shapes against
// the independent bytecode record of the reference probe (slice V0): every
// class the manifest records has the same overriding methods, each with the
// same parent calls and argument shapes.
func TestHooksMatchProbeManifest(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "gameserver", "script", "testdata", "oracle", "manifest.golden"))
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	defer f.Close()

	manifest := map[string][]string{}
	var class string
	inClasses := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "# classes":
			inClasses = true
		case strings.HasPrefix(line, "# "):
			inClasses = false
		case !inClasses:
		case strings.HasPrefix(line, "class "):
			// Engine classes in a chain (ScheduledQuest, the clan hall siege
			// base) are not script classes and have no fingerprint.
			class = strings.Fields(line)[1]
			if slices.ContainsFunc(scriptTrees, func(tree string) bool {
				return strings.HasPrefix(class, strings.TrimSuffix(tree, "/")+".")
			}) {
				manifest[class] = []string{}
			} else {
				class = ""
			}
		case class == "":
		default:
			m := manifestHook.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			shape := "none"
			var args []string
			for _, s := range manifestSuper.FindAllStringSubmatch(line, -1) {
				args = append(args, s[1])
			}
			if len(args) > 0 {
				shape = "direct " + strings.Join(args, ",")
			}
			manifest[class] = append(manifest[class], m[1]+" "+shape)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if len(manifest) < 857 {
		t.Fatalf("manifest records %d classes, want at least the 857 listed scripts", len(manifest))
	}

	ref, err := Reference()
	if err != nil {
		t.Fatalf("Reference: %v", err)
	}
	hooks := 0
	for class, want := range manifest {
		fp, ok := ref[class]
		if !ok {
			t.Errorf("%s: in the manifest, no fingerprint", class)
			continue
		}
		var got []string
		for _, h := range fp.Hooks {
			got = append(got, h.String())
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: hooks %q, manifest %q", class, got, want)
		}
		hooks += len(want)
	}
	if hooks < 2600 {
		t.Errorf("compared %d hooks, want the manifest's ~2,675", hooks)
	}
}

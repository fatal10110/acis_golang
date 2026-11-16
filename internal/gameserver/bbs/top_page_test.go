package bbs

import (
	"os"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestTopPageShippedPages accepts every page the datapack ships in the home
// folder by its file name.
func TestTopPageShippedPages(t *testing.T) {
	entries, err := os.ReadDir(datapack.Path(t, "data", "html", "CommunityBoard", "top"))
	if err != nil {
		t.Fatal(err)
	}
	pages := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		pages++
		got, ok := TopPage(e.Name())
		if !ok || got != TopFolder+e.Name() {
			t.Errorf("TopPage(%q) = %q, %v; want %q, true", e.Name(), got, ok, TopFolder+e.Name())
		}
	}
	if pages == 0 {
		t.Fatal("no shipped home pages")
	}
}

// TestTopPageRefusesPaths refuses a token that is not a plain file name in
// the home folder (owner-approved narrowing, #4708).
func TestTopPageRefusesPaths(t *testing.T) {
	for _, token := range []string{
		"", ".", "..",
		"../../admin/secret.htm", `..\..\admin\secret.htm`,
		"a/b", `a\b`, "/news.htm", `\news.htm`, "./news.htm", "news.htm/",
	} {
		if got, ok := TopPage(token); ok {
			t.Errorf("TopPage(%q) = %q, true; want refused", token, got)
		}
	}
}

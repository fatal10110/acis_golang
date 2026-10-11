package cache

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestLoadHTMLLeavesOutUndecodablePages: a page file that is not valid
// UTF-8 (a stray Latin-1 byte, an overlong form, an encoded surrogate, a
// truncated sequence) cannot be read as a page, so Get reports it missing
// and LoadHTML names it; valid multi-byte text is kept.
func TestLoadHTMLLeavesOutUndecodablePages(t *testing.T) {
	dir := t.TempDir()
	writeHTML(t, dir, "ok.htm", "<html>café 中</html>")
	writeHTML(t, dir, "q/latin1.htm", "<html>caf\xe9</html>")
	writeHTML(t, dir, "q/overlong.htm", "<html>\xc0\xaf</html>")
	writeHTML(t, dir, "q/surrogate.htm", "<html>\xed\xa0\x80</html>")
	writeHTML(t, dir, "a/truncated.html", "<html>\xe4\xb8")

	cache, undecodable, err := LoadHTML(dir)
	if err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	if want := []string{"a/truncated.html", "q/latin1.htm", "q/overlong.htm", "q/surrogate.htm"}; !slices.Equal(undecodable, want) {
		t.Fatalf("undecodable = %q, want %q", undecodable, want)
	}
	for _, name := range undecodable {
		if _, ok := cache.Get(name); ok {
			t.Errorf("Get(%q) ok = true, want missing", name)
		}
	}
	if got, ok := cache.Get("ok.htm"); !ok || got != "<html>café 中</html>\n" {
		t.Fatalf("Get(ok.htm) = %q, %t", got, ok)
	}
	if got := cache.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
}

// TestLoadHTMLDatapackUndecodablePages: the datapack's page files a strict
// UTF-8 line reader rejects with MalformedInputException, found by reading
// every .htm/.html file under data/html that way (15320 files, 15308
// read). Each is missing from the cache.
func TestLoadHTMLDatapackUndecodablePages(t *testing.T) {
	dir := datapack.Path(t, "data", "html")

	cache, undecodable, err := LoadHTML(dir)
	if err != nil {
		t.Fatalf("LoadHTML(%q): %v", dir, err)
	}
	want := []string{
		"script/quest/Q114_ResurrectionOfAnOldManager/32041-04.htm",
		"script/quest/Q114_ResurrectionOfAnOldManager/32041-26.htm",
		"script/quest/Q114_ResurrectionOfAnOldManager/32047-33.htm",
		"script/quest/Q114_ResurrectionOfAnOldManager/32047-37.htm",
		"script/quest/Q123_TheLeaderAndTheFollower/31961-02.htm",
		"script/quest/Q123_TheLeaderAndTheFollower/31961-14.htm",
		"script/quest/Q215_TrialOfThePilgrim/30649-04.htm",
		"script/quest/Q336_CoinsOfMagic/30232-13.htm",
		"script/quest/Q336_CoinsOfMagic/30232-14.htm",
		"script/quest/Q617_GatherTheFlames/31271-01.htm",
		"script/quest/Q617_GatherTheFlames/31271-02.htm",
		"script/siegablehall/BanditStronghold/agit_oel_mahum_messeger_20.htm",
	}
	if !slices.Equal(undecodable, want) {
		t.Fatalf("undecodable =\n%q\nwant\n%q", undecodable, want)
	}
	for _, name := range want {
		if _, ok := cache.Get("./data/html/" + name); ok {
			t.Errorf("Get(%q) ok = true, want missing", name)
		}
	}
	// A sibling page of the same quest that is valid UTF-8 stays loaded.
	if _, ok := cache.Get("script/quest/Q123_TheLeaderAndTheFollower/31961-01.htm"); !ok {
		t.Fatal("Get(Q123 31961-01.htm) ok = false, want loaded")
	}
}

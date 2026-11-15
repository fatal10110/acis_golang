package cache

import "testing"

// TestHTMLGetRefusesClimbingNames pins that a name with a ".." segment
// names no page, even when it would climb to a loaded one.
func TestHTMLGetRefusesClimbingNames(t *testing.T) {
	dir := t.TempDir()
	writeHTML(t, dir, "chamberlain/manor/manor_help001.htm", "<html>help</html>")
	writeHTML(t, dir, "secret.htm", "<html>secret</html>")
	cache, _, err := LoadHTML(dir)
	if err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for _, name := range []string{
		"chamberlain/manor/manor_help00../../../secret.htm",
		"data/html/chamberlain/../secret.htm",
		`chamberlain\..\secret.htm`,
		"../html/secret.htm",
		"..",
	} {
		if got, ok := cache.Get(name); ok {
			t.Fatalf("Get(%q) = %q, want no page", name, got)
		}
	}
	for _, name := range []string{"secret.htm", "./data/html/chamberlain/manor/manor_help001.htm", "chamberlain/./manor/manor_help001.htm"} {
		if _, ok := cache.Get(name); !ok {
			t.Fatalf("Get(%q) found no page", name)
		}
	}
}

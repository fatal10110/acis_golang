package cache

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeHTML(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadHTML(t *testing.T) {
	dir := t.TempDir()
	const content = "<html>\r\n<body>keep\tme</body>\r</html>"
	const want = "<html>\n<body>keep\tme</body>\n</html>\n"
	writeHTML(t, dir, "merchant/shop.htm", content)
	writeHTML(t, dir, "merchant/extra.html", "<html>extra</html>")
	writeHTML(t, dir, "readme.txt", "ignored")

	cache, err := LoadHTML(dir)
	if err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	if cache.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", cache.Len())
	}

	for _, name := range []string{"merchant/shop.htm", "data/html/merchant/shop.htm", "./data/html/merchant/shop.htm", `data\html\merchant\shop.htm`} {
		got, ok := cache.Get(name)
		if !ok {
			t.Fatalf("Get(%q) ok = false", name)
		}
		if got != want {
			t.Fatalf("Get(%q) = %q, want %q", name, got, want)
		}
	}
	if got, ok := cache.Get("merchant/extra.html"); !ok || got != "<html>extra</html>\n" {
		t.Fatalf("Get(merchant/extra.html) = %q, %t; want %q, true", got, ok, "<html>extra</html>\n")
	}
	if _, ok := cache.Get("merchant/missing.htm"); ok {
		t.Fatal("Get(missing) ok = true, want false")
	}
}

func TestLoadHTMLMissingAndEmpty(t *testing.T) {
	if _, err := LoadHTML(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("LoadHTML(missing) error = nil")
	}
	if _, err := LoadHTML(t.TempDir()); err == nil {
		t.Fatal("LoadHTML(empty) error = nil")
	}
}

func TestHTMLPathsAreSorted(t *testing.T) {
	dir := t.TempDir()
	writeHTML(t, dir, "z.htm", "z")
	writeHTML(t, dir, "a/b.htm", "b")

	cache, err := LoadHTML(dir)
	if err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	if got, want := cache.Paths(), []string{"a/b.htm", "z.htm"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Paths() = %#v, want %#v", got, want)
	}
}

func TestBypassCommands(t *testing.T) {
	html := `<a action="bypass -h npc_%objectId%_Chat 1">Talk</a>` +
		`<a action="bypass player_help tutorial.htm#7064">Help</a>` +
		`<a action="bypass -h npc_$ask">Ask</a>`

	got := BypassCommands(html)
	want := []string{"npc_%objectId%_Chat 1", "player_help tutorial.htm#7064", "npc_"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BypassCommands() = %#v, want %#v", got, want)
	}
}

func TestLoadHTMLAgainstDatapack(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "..", "aCis_datapack", "data", "html")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("aCis_datapack not checked out next to module root, skipping: %v", err)
	}

	cache, err := LoadHTML(dir)
	if err != nil {
		t.Fatalf("LoadHTML(%q): %v", dir, err)
	}
	if got, want := cache.Len(), 15320; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}

	content, ok := cache.Get("territorynoclan.htm")
	if !ok {
		t.Fatal("Get(territorynoclan.htm) ok = false")
	}
	if strings.Contains(content, "\r") || !strings.HasSuffix(content, "\n") {
		t.Fatalf("territorynoclan.htm content = %q, want LF-only content ending in newline", content)
	}
}

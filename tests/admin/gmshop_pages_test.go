package admin

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

var adminLinkTarget = regexp.MustCompile(`admin_link ([\w/.]+)`)

// TestGMShopPagesResolve opens the shipped GM shop index and follows every
// admin_link it and its pages hold: each target is served as a page, none is
// the missing-page notice, and the walk reaches every page in gmshop/, the
// index's three content pages and nothing else.
func TestGMShopPagesResolve(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(datapack.Path(t, "data", "html", "admin", "gmshop"))
	if err != nil {
		t.Fatalf("read gmshop dir: %v", err)
	}
	shipped := []string{"gmshops.htm"}
	for _, e := range entries {
		shipped = append(shipped, "gmshop/"+e.Name())
	}
	slices.Sort(shipped)
	if want := []string{"gmshop/armor.htm", "gmshop/items.htm", "gmshop/weapon.htm", "gmshops.htm"}; !slices.Equal(shipped, want) {
		t.Fatalf("shipped GM shop pages = %v, want %v", shipped, want)
	}

	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, shipped...)))
	c := srv.Client
	enterWorld(t, c)

	visited := map[string]bool{}
	queue := []string{"gmshops.htm"}
	for len(queue) > 0 {
		page := queue[0]
		queue = queue[1:]
		if visited[page] {
			continue
		}
		visited[page] = true
		frames := exchange(t, c, encodeBuildCmd("link "+page))
		assertPage(t, frames, "GM Shop")
		body := htmlBody(t, frames[0])
		if strings.Contains(body, "My html is missing") {
			t.Fatalf("//link %s answered the missing-page notice", page)
		}
		for _, m := range adminLinkTarget.FindAllStringSubmatch(body, -1) {
			queue = append(queue, m[1])
		}
	}
	reached := make([]string, 0, len(visited))
	for page := range visited {
		reached = append(reached, page)
	}
	slices.Sort(reached)
	if !slices.Equal(reached, shipped) {
		t.Fatalf("pages reached from gmshops.htm = %v, want every shipped page %v", reached, shipped)
	}
}

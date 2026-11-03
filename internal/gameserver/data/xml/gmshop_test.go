package xml

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
)

var gmShopBuy = regexp.MustCompile(`admin_buy (\d+)`)

// TestGMShopBuyListsAgainstDatapack pins the shipped GM shop pages to the
// shipped buylists: every admin_buy id the pages hold names a non-empty
// GM-only list (npcId -1), and every GM-only list is offered by a page.
func TestGMShopBuyListsAgainstDatapack(t *testing.T) {
	t.Parallel()
	xmlDir := datapackPath(t, filepath.Join("data", "xml"))
	adminDir := datapackPath(t, filepath.Join("data", "html", "admin"))

	items, err := LoadItemTemplates(filepath.Join(xmlDir, "items"), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadItemTemplates error: %v", err)
	}
	table, err := LoadBuyLists(filepath.Join(xmlDir, "buyLists.xml"), items)
	if err != nil {
		t.Fatalf("LoadBuyLists error: %v", err)
	}

	offered := map[int]bool{}
	for _, page := range []string{"gmshops.htm", "gmshop/weapon.htm", "gmshop/armor.htm", "gmshop/items.htm"} {
		raw, err := os.ReadFile(filepath.Join(adminDir, filepath.FromSlash(page)))
		if err != nil {
			t.Fatalf("read %s: %v", page, err)
		}
		for _, m := range gmShopBuy.FindAllStringSubmatch(string(raw), -1) {
			id, _ := strconv.Atoi(m[1])
			list, ok := table.Find(id)
			if !ok || list.NPCID != -1 || len(list.Products) == 0 {
				t.Errorf("%s: admin_buy %d = %+v, %v; want a non-empty GM-only buylist", page, id, list, ok)
			}
			offered[id] = true
		}
	}
	if got, want := len(offered), 188; got != want {
		t.Errorf("distinct buylists offered = %d, want %d", got, want)
	}

	var unoffered []int
	table.Each(func(l buylist.List) {
		if l.NPCID == -1 && !offered[l.ID] {
			unoffered = append(unoffered, l.ID)
		}
	})
	slices.Sort(unoffered)
	if len(unoffered) != 0 {
		t.Fatalf("GM-only buylists no GM shop page offers: %v", unoffered)
	}
}

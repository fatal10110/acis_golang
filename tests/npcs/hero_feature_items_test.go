package npcs

import (
	"os"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/herocirclet"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/heroweapon"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The hero's weapon and circlet (HeroWeapon, HeroCirclet) at a Monument of
// Heroes. The hero's paths are the scenarios hero_items_hero and
// hero_items_non_hero; here is the refusal page a non-hero is shown, which no
// monument page offers a link to (the bypass check refuses it first).

const (
	heroCirclet     = 6842
	infinityBlade   = 6611
	infinityCleaver = 6612
	heroWeaponDir   = "script/feature/HeroWeapon/"
	heroCircletDir  = "script/feature/HeroCirclet/"
)

// featurePages returns the datapack pages of the feature directories, keyed
// by their path under data/html.
func featurePages(t *testing.T, names ...string) map[string]string {
	t.Helper()
	datapack.Require(t)
	pages := map[string]string{}
	for _, name := range names {
		dir := datapack.Path(t, "data", "html", "script", "feature", name)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			data, err := os.ReadFile(datapack.Path(t, "data", "html", "script", "feature", name, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			pages["script/feature/"+name+"/"+e.Name()] = string(data)
		}
	}
	return pages
}

// heroFeatureOptions registers both features on the Monument of Heroes,
// with their pages, the page that admits every command and the hero items'
// templates.
func heroFeatureOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	pages := featurePages(t, "HeroWeapon", "HeroCirclet")
	pages["test/any.htm"] = anyNpcPage
	templates := gameservertest.ItemTemplates().All()
	for id := int32(6611); id <= 6621; id++ {
		templates = append(templates, &item.Template{
			ID: id, Name: "Weapon " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	templates = append(templates, &item.Template{
		ID: heroCirclet, Name: "Circlet", Kind: item.KindEtcItem, Duration: -1,
		Destroyable: true, EtcItem: &item.EtcItemDetail{},
	})
	return []gameservertest.Option{
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{monumentID: script.KindFolk},
			[]script.Listing{{Path: "script.feature.HeroCirclet"}, {Path: "script.feature.HeroWeapon"}},
			script.Catalog{"script.feature.HeroCirclet": herocirclet.New, "script.feature.HeroWeapon": heroweapon.New}),
	}
}

// TestHeroItemsRefusedToANonHero: a character that is not a hero is told so
// by both features, and given nothing.
func TestHeroItemsRefusedToANonHero(t *testing.T) {
	t.Parallel()
	w, monument := bootMonument(t, 76, heroFeatureOptions(t)...)

	w.openAnyNpcPage(t)
	wantPage(t, w.bypass(t, npcCommand(monument, "Quest HeroWeapon")), monument, scriptPage(t, heroWeaponDir+"no_hero.htm", monument), 2)
	w.openAnyNpcPage(t)
	wantPage(t, w.bypass(t, npcCommand(monument, "Quest HeroCirclet")), monument, scriptPage(t, heroCircletDir+"no_hero.htm", monument), 2)
	if n := w.srv.PlayerItemCount(t, w.player, heroCirclet); n != 0 {
		t.Fatalf("circlets held = %d, want 0", n)
	}
}

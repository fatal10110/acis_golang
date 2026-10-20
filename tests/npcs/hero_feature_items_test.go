package npcs

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/herocirclet"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/heroweapon"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The hero's weapon and circlet (HeroWeapon, HeroCirclet) at a Monument of
// Heroes. A scenario cannot seed the hero status, so the hero's paths are
// here: the monument's hero page links both features.

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

// openHeroPage talks to the monument and requires its hero page, which
// links both features.
func (w *folkWorld) openHeroPage(t *testing.T, monument *npc.Folk) {
	t.Helper()
	w.selectFolk(t, monument)
	frame, ok := firstOpcode(w.talk(t, monument, false), serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatal("talk opened no page")
	}
	_, html, _ := htmlMessage(t, frame)
	for _, link := range []string{"Quest HeroWeapon", "Quest HeroCirclet"} {
		if !strings.Contains(html, npcCommand(monument, link)) {
			t.Fatalf("monument page %q lacks %q", html, link)
		}
	}
}

// wantGiven requires frames to be one item-obtained message of itemID and
// nothing else of the dialog: no page, no release.
func wantGiven(t *testing.T, frames [][]byte, itemID int32) {
	t.Helper()
	got := dialogFrames(frames)
	if !slices.Equal(opcodes(got), []byte{serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("answer = %x, want only the item message", opcodes(got))
	}
	if ids := systemMessageIDs(got); !slices.Equal(ids, []int32{54}) {
		t.Fatalf("messages = %v, want the obtained-item message 54 for %d", ids, itemID)
	}
}

// TestHeroWeaponForAHero: a hero holding no hero's weapon opens the weapon
// list from the monument's hero page and picks the Infinity Blade, which it
// is given with no page; the monument then says it has one. The list's
// other links, still open, give the weapon they name, as a pick answers no
// page (the reference's behavior; #4076 asks whether to refuse it).
func TestHeroWeaponForAHero(t *testing.T) {
	t.Parallel()
	w, monument := bootMonument(t, 76, append(heroFeatureOptions(t), heroSeed(t, true))...)

	w.openHeroPage(t, monument)
	wantPage(t, w.bypass(t, npcCommand(monument, "Quest HeroWeapon")), monument, scriptPage(t, heroWeaponDir+"weapon_list.htm", monument), 2)
	wantGiven(t, w.bypass(t, "Quest HeroWeapon "+strconv.Itoa(infinityBlade)), infinityBlade)
	if n := w.srv.PlayerItemCount(t, w.player, infinityBlade); n != 1 {
		t.Fatalf("Infinity Blade held = %d, want 1", n)
	}
	wantGiven(t, w.bypass(t, "Quest HeroWeapon "+strconv.Itoa(infinityCleaver)), infinityCleaver)
	if n := w.srv.PlayerItemCount(t, w.player, infinityCleaver); n != 1 {
		t.Fatalf("Infinity Cleaver held = %d, want 1", n)
	}

	w.openHeroPage(t, monument)
	wantPage(t, w.bypass(t, npcCommand(monument, "Quest HeroWeapon")), monument, scriptPage(t, heroWeaponDir+"already_have_weapon.htm", monument), 2)
}

// TestHeroWeaponEvents: an event naming no hero's weapon gives nothing, and
// one that is not a number aborts; neither answers anything.
func TestHeroWeaponEvents(t *testing.T) {
	t.Parallel()
	w, monument := bootMonument(t, 76, append(heroFeatureOptions(t), heroSeed(t, true))...)

	w.openHeroPage(t, monument)
	wantPage(t, w.bypass(t, npcCommand(monument, "Quest HeroWeapon")), monument, scriptPage(t, heroWeaponDir+"weapon_list.htm", monument), 2)
	w.openAnyNpcPage(t)
	for _, event := range []string{"6610", "6622", "57", "abc", "4294973907", ""} {
		w.c.Send(encodeBypass("Quest HeroWeapon " + event))
		if got := dialogFrames(w.srv.ReadQueued(t, w.c)); len(got) != 0 {
			t.Fatalf("%q answered %x, want nothing", event, opcodes(got))
		}
	}
	for id := int32(6611); id <= 6621; id++ {
		if n := w.srv.PlayerItemCount(t, w.player, id); n != 0 {
			t.Fatalf("weapon %d held = %d, want 0", id, n)
		}
	}
}

// TestHeroCircletForAHero: a hero without the circlet is given one from the
// monument's hero page, with no page; asked again, the monument says it
// has it.
func TestHeroCircletForAHero(t *testing.T) {
	t.Parallel()
	w, monument := bootMonument(t, 76, append(heroFeatureOptions(t), heroSeed(t, true))...)

	w.openHeroPage(t, monument)
	frames := w.bypass(t, npcCommand(monument, "Quest HeroCirclet"))
	got := dialogFrames(frames)
	if !slices.Equal(opcodes(got), []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("answer = %x, want the item message then the release", opcodes(got))
	}
	if ids := systemMessageIDs(got); !slices.Equal(ids, []int32{54}) {
		t.Fatalf("messages = %v, want the obtained-item message 54", ids)
	}
	if n := w.srv.PlayerItemCount(t, w.player, heroCirclet); n != 1 {
		t.Fatalf("circlets held = %d, want 1", n)
	}

	w.openHeroPage(t, monument)
	wantPage(t, w.bypass(t, npcCommand(monument, "Quest HeroCirclet")), monument, scriptPage(t, heroCircletDir+"already_have_circlet.htm", monument), 2)
	if n := w.srv.PlayerItemCount(t, w.player, heroCirclet); n != 1 {
		t.Fatalf("circlets held = %d, want 1", n)
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

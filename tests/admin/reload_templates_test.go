package admin

import (
	"slices"
	"strings"
	"sync"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// reloadedSkillID is a skill only the reloaded skill table defines.
const reloadedSkillID = 9001

// dropItemTable is the shared item catalog plus infoWolf's drop items, its
// first drop (item 1003) named pelt.
func dropItemTable(pelt string) *item.Table {
	templates := gameservertest.ItemTemplates().All()
	for id, name := range map[int32]string{
		infoItemLong:   "Sealed Tallum Tunic of the Ancient Kingdom Masterwork",
		infoItemRecipe: "Recipe: Soulshot (D)",
		1003:           pelt, 1004: "Wolf Fang", 1005: "Animal Bone", 1006: "Thread", 1007: "Coal", 1008: "Stem",
	} {
		templates = append(templates, &item.Template{ID: id, Name: name, Kind: item.KindEtcItem, Duration: -1, EtcItem: &item.EtcItemDetail{}})
	}
	return item.NewTable(templates)
}

// peltRow is the first row of infoWolf's drop page, its pelt named name.
func peltRow(name string) string {
	return dropRow(0, name, "90EE90", "90", "1")
}

// reloadSkillTable is the //skill fixture's table, plus reloadedSkillID
// when reloaded is set.
func reloadSkillTable(reloaded bool) *modelskill.Table {
	defs := []modelskill.Definition{{ID: boughtSkill, Level: 1, Name: "Bought"}, {ID: freeSkill, Level: 1, Name: "Free"}}
	if reloaded {
		defs = append(defs, modelskill.Definition{ID: reloadedSkillID, Level: 1, Name: "Reloaded"}, modelskill.Definition{ID: reloadedSkillID, Level: 2, Name: "Reloaded"})
	}
	return modelskill.NewTable(defs)
}

// bootTemplateReload boots the GM with its own item and skill tables and
// reloads, spawns infoWolf and selects it, returning the wolf's object id.
func bootTemplateReload(t *testing.T, items *item.Table, skills *modelskill.Table, reloads network.DataReloads) (*gameservertest.Server, int32) {
	t.Helper()
	db := sqltest.SharedDB(t)
	persistence := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), skills, gamesql.NewCharacterSkillStore(db))
	pages := append([]string{"char_skills.htm", "char_skills_list.htm"}, infoNpcPages...)
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithItemTemplates(items),
		gameservertest.WithSkills(persistence),
		gameservertest.WithHTMLPages(shippedAdminPages(t, pages...)),
		gameservertest.WithNpcDropRates(item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}),
		gameservertest.WithDataReloads(reloads),
	)
	gm := srv.Client
	enterWorld(t, gm)
	wolf := srv.SpawnHostileNPCTemplateAt(t, infoWolf(), location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
	drain(t, gm)
	exchange(t, gm, encodeAction(wolf.ObjectID()))
	return srv, wolf.ObjectID()
}

// TestAdminReloadItemAndSkillTemplates pins //reload item and //reload
// skill (AdminReload.java "item" and "skill"): each reads its templates
// anew and swaps them in for every reader, telling the GM so, and the next
// lookup reads the reloaded ones — the drop page names the reloaded item,
// and //skill set finds a skill only the reloaded table defines.
func TestAdminReloadItemAndSkillTemplates(t *testing.T) {
	t.Parallel()
	items := dropItemTable("Wolf Pelt")
	skills := reloadSkillTable(false)
	srv, _ := bootTemplateReload(t, items, skills, network.DataReloads{
		Items: func() error {
			items.Replace(dropItemTable("Wolf Fur"))
			return nil
		},
		Skills: func() error {
			skills.Replace(reloadSkillTable(true))
			return nil
		},
	})
	gm := srv.Client

	if page := infoPage(t, gm, "info drop"); !strings.Contains(page, peltRow("Wolf Pelt")) {
		t.Fatalf("drop page before the reload = %q, want the booted pelt name", page)
	}
	if got := textsIn(t, exchange(t, gm, encodeBuildCmd("skill set 9001 1"))); !slices.Equal(got, []string{"Usage: //skill set id level [page]"}) {
		t.Fatalf("//skill set of an unknown skill = %q, want the usage", got)
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload items skills")), "Items' templates have been reloaded.", "Skills' XMLs have been reloaded.")

	if page := infoPage(t, gm, "info drop"); !strings.Contains(page, peltRow("Wolf Fur")) {
		t.Fatalf("drop page after the reload = %q, want the reloaded pelt name", page)
	}
	if got := textsIn(t, exchange(t, gm, encodeBuildCmd("skill set 9001 2"))); !slices.Contains(got, "You gave Reloaded skill to Admin.") {
		t.Fatalf("//skill set of the reloaded skill = %q, want it given", got)
	}
	if level := storedSkills(t, srv, srv.SoleObjectID(t))[reloadedSkillID]; level != 2 {
		t.Fatalf("stored level of the reloaded skill = %d, want 2", level)
	}
}

// TestAdminReloadTemplatesRaceFree reloads the item and skill templates
// through one game master while another reads both through its own
// commands: under -race, every read sees one table or the other and
// nothing races.
func TestAdminReloadTemplatesRaceFree(t *testing.T) {
	t.Parallel()
	items := dropItemTable("Wolf Pelt")
	skills := reloadSkillTable(false)
	var flip bool // only the reloading game master's queue runs the hooks
	srv, wolfObjID := bootTemplateReload(t, items, skills, network.DataReloads{
		Items: func() error {
			name := "Wolf Fur"
			if flip {
				name = "Wolf Pelt"
			}
			items.Replace(dropItemTable(name))
			return nil
		},
		Skills: func() error {
			skills.Replace(reloadSkillTable(!flip))
			flip = !flip
			return nil
		},
	})
	gm := srv.Client
	reader, _ := addPlayer(t, srv, "reader", "Reader", adminLevel)
	drain(t, reader)
	exchange(t, reader, encodeAction(wolfObjID))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 10 {
			frames := exchange(t, reader, encodeBuildCmd("info drop"))
			if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeNpcHtmlMessage {
				t.Errorf("//info drop during reloads answered %x", testsupport.FrameOpcodes(frames))
				return
			}
			if page := htmlBody(t, frames[0]); !strings.Contains(page, peltRow("Wolf Pelt")) && !strings.Contains(page, peltRow("Wolf Fur")) {
				t.Errorf("//info drop during reloads = %q, want either pelt name", page)
				return
			}
			for _, frame := range exchange(t, reader, encodeBuildCmd("skill list")) {
				if frame[0] != serverpackets.OpcodeNpcHtmlMessage {
					t.Errorf("//skill list during reloads answered %#x", frame[0])
					return
				}
			}
		}
	}()
	want := []string{"Items' templates have been reloaded.", "Skills' XMLs have been reloaded."}
	for range 10 {
		if got := textsIn(t, exchange(t, gm, encodeBuildCmd("reload item skill"))); !slices.Equal(got, want) {
			t.Fatalf("//reload during reads answered %q, want %q", got, want)
		}
	}
	wg.Wait()
}

package admin

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The shipped Mithril Heavy Set (armorSets.xml), as tests/items rigs it:
// chest 58, legs 59, head 47, set skill 3502, +6 skill 3611. Each granted
// skill carries its own maxHp bonus so a grant or removal shows in MaxHP.
const (
	mithrilChestID    int32 = 58
	mithrilLegsID     int32 = 59
	mithrilHeadID     int32 = 47
	armorSetCommonID  int32 = 3006
	mithrilSetSkillID int32 = 3502
	mithrilEnchant6ID int32 = 3611
	expertiseSkillID        = 239
	mithrilEnchant6HP       = 1000
)

var mithrilSlots = map[int32]int{
	mithrilChestID: itemcontainer.Chest,
	mithrilLegsID:  itemcontainer.Legs,
	mithrilHeadID:  itemcontainer.Head,
}

var shippedItemTemplates = sync.OnceValues(func() (*item.Table, error) {
	dir, _ := datapack.Find()
	return xmldata.LoadItemTemplates(filepath.Join(dir, "data", "xml", "items"), zerolog.Nop())
})

// bootArmorSetAdmin boots the GM "Admin" against the shipped armor set
// table, wearing the Mithril chest, legs and head at the given enchant
// levels, and enters the world. It returns the server and the GM's id.
func bootArmorSetAdmin(t *testing.T, enchant map[int32]int) (*gameservertest.Server, int32) {
	t.Helper()
	dir := datapack.Require(t)
	shipped, err := shippedItemTemplates()
	if err != nil {
		t.Fatalf("load item templates: %v", err)
	}
	sets, err := xmldata.LoadArmorSets(filepath.Join(dir, "data", "xml", "armorSets.xml"))
	if err != nil {
		t.Fatalf("load armor sets: %v", err)
	}
	templates := gameservertest.ItemTemplates().All()
	for id := range mithrilSlots {
		tmpl, ok := shipped.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	passive := func(id int32, hp int) modelskill.Definition {
		return modelskill.Definition{ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "maxHp", Value: float64(hp)},
		}}
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: expertiseSkillID, Level: 1},
		passive(armorSetCommonID, 1),
		passive(mithrilSetSkillID, 10),
		passive(mithrilEnchant6ID, mithrilEnchant6HP),
	}), gamesql.NewCharacterSkillStore(db))
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithSkills(skills),
		gameservertest.WithArmorSets(sets))
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), gmID, 0, expertiseSkillID, 1); err != nil {
		t.Fatalf("grant Expertise: %v", err)
	}
	for id, slot := range mithrilSlots {
		row := itemRow(t, srv, gmID, srv.GiveItem(t, gmID, id, 1))
		row.EnchantLevel = enchant[id]
		row.Location, row.LocationData = item.LocationPaperdoll, slot
		if err := srv.Items.Update(context.Background(), row); err != nil {
			t.Fatalf("wear item %d: %v", id, err)
		}
	}
	enterWorld(t, srv.Client)
	drain(t, srv.Client)
	return srv, gmID
}

// statRefreshThenSkillList is what the GM sees when the +6 skill is granted
// or revoked: the skill's maxHp func changing the stats refreshes UserInfo
// first (Creature.broadcastModifiedStats), then the SkillList, the
// command's own UserInfo and the report.
var statRefreshThenSkillList = []byte{
	serverpackets.OpcodeUserInfo, serverpackets.OpcodeSkillList,
	serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage,
}

// TestAdminEnchantArmorSet pins the armor branch of //enchant
// (AdminEnchant.java): a worn armor piece leaving +6 revokes the +6 skill
// of the set the worn chest belongs to and resends SkillList; reaching +6
// grants it only once every piece of that set is at +6 or higher; either
// way UserInfo and the GM's report follow.
func TestAdminEnchantArmorSet(t *testing.T) {
	t.Parallel()

	t.Run("+6 to +5 revokes the +6 skill", func(t *testing.T) {
		t.Parallel()
		srv, gmID := bootArmorSetAdmin(t, map[int32]int{mithrilChestID: 6, mithrilLegsID: 6, mithrilHeadID: 6})
		before := srv.PlayerMaxHP(t, gmID)
		frames := exchange(t, srv.Client, encodeBuildCmd("enchant chest 5"))
		testsupport.AssertOpcodeSequence(t, frames, statRefreshThenSkillList...)
		if skillListHas(frames[1], mithrilEnchant6ID) {
			t.Fatal("SkillList after //enchant chest 5 still lists the +6 skill")
		}
		if !skillListHas(frames[1], mithrilSetSkillID) {
			t.Fatal("SkillList after //enchant chest 5 lost the set skill")
		}
		assertTexts(t, frames[3:], "Admin's Mithril Breastplate enchant was modified from 6 to 5.")
		if got, want := srv.PlayerMaxHP(t, gmID), before-mithrilEnchant6HP; got != want {
			t.Fatalf("MaxHP after the +6 revoke = %d, want %d", got, want)
		}
	})

	t.Run("+5 to +6 with another piece below +6 grants nothing", func(t *testing.T) {
		t.Parallel()
		srv, gmID := bootArmorSetAdmin(t, map[int32]int{mithrilChestID: 5, mithrilLegsID: 5, mithrilHeadID: 6})
		before := srv.PlayerMaxHP(t, gmID)
		frames := exchange(t, srv.Client, encodeBuildCmd("enchant chest 6"))
		testsupport.AssertOpcodeSequence(t, frames, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage)
		assertTexts(t, frames[1:], "Admin's Mithril Breastplate enchant was modified from 5 to 6.")
		if got := srv.PlayerMaxHP(t, gmID); got != before {
			t.Fatalf("MaxHP after the chest reached +6 with the legs at +5 = %d, want %d", got, before)
		}
	})

	t.Run("+5 to +6 completing the set grants the +6 skill", func(t *testing.T) {
		t.Parallel()
		srv, gmID := bootArmorSetAdmin(t, map[int32]int{mithrilChestID: 6, mithrilLegsID: 7, mithrilHeadID: 5})
		before := srv.PlayerMaxHP(t, gmID)
		frames := exchange(t, srv.Client, encodeBuildCmd("enchant head 6"))
		testsupport.AssertOpcodeSequence(t, frames, statRefreshThenSkillList...)
		if !skillListHas(frames[1], mithrilEnchant6ID) {
			t.Fatal("SkillList after //enchant head 6 lacks the +6 skill")
		}
		assertTexts(t, frames[3:], "Admin's Helmet enchant was modified from 5 to 6.")
		if got, want := srv.PlayerMaxHP(t, gmID), before+mithrilEnchant6HP; got != want {
			t.Fatalf("MaxHP after the +6 grant = %d, want %d", got, want)
		}
	})
}

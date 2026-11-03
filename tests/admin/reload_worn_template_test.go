package admin

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The rigged weapon of the reload tests below: a copy of the shared sword
// that only the booted item table defines, adding max HP itself, with an
// attached skill and a +4 enchant skill that each add their own. Worn at +4
// with riggedAugmentation, whose stat bonus and red augmentation skill add
// max HP too.
const (
	riggedWeaponID       int32 = 65013
	riggedWeaponHP             = 100
	riggedItemSkillID    int32 = 3099
	riggedItemSkillHP          = 10
	riggedEnchantSkillID int32 = 3098
	riggedEnchantSkillHP       = 50
	riggedAugmentSkillHP       = 20
	// riggedAugmentation is the red skill option of life stone level 0
	// over the solo maxHp option of the first color block, which adds
	// riggedAugmentStatHP.
	riggedAugmentation  int32 = 14563<<16 | 3
	riggedAugmentStatHP       = 30
)

// riggedWeaponTable is the shared catalog plus the rigged weapon when
// withWeapon is set. Each call builds new templates, as a reload does.
func riggedWeaponTable(withWeapon bool) *item.Table {
	templates := gameservertest.ItemTemplates().All()
	if withWeapon {
		sword, _ := gameservertest.ItemTemplates().Get(swordID)
		rigged := *sword
		weapon := *sword.Weapon
		weapon.Enchant4Skill = &item.SkillRef{ID: riggedEnchantSkillID, Level: 1}
		rigged.ID = riggedWeaponID
		rigged.Weapon = &weapon
		rigged.Modifiers = []item.StatModifier{{Op: item.FuncAdd, Stat: "maxHp", Value: riggedWeaponHP}}
		rigged.AttachedSkills = []item.SkillRef{{ID: riggedItemSkillID, Level: 1}}
		templates = append(templates, &rigged)
	}
	return item.NewTable(templates)
}

// lastFrameOf is the last frame among frames with the given opcode.
func lastFrameOf(t *testing.T, frames [][]byte, opcode byte) []byte {
	t.Helper()
	for i := len(frames) - 1; i >= 0; i-- {
		if len(frames[i]) > 0 && frames[i][0] == opcode {
			return frames[i]
		}
	}
	t.Fatalf("no frame with opcode %#x among %d", opcode, len(frames))
	return nil
}

// TestReloadedAwayWornWeaponUnequipTakesOffWhatItBrought wears the rigged
// weapon, reloads the item table without it and takes it off: the attached
// skill, the +4 enchant skill and the augmentation's skill leave the skill
// list, and every max HP bonus it brought, the augmentation's included,
// leaves UserInfo.
func TestReloadedAwayWornWeaponUnequipTakesOffWhatItBrought(t *testing.T) {
	t.Parallel()
	items := riggedWeaponTable(true)
	passive := func(id int32, hp int) modelskill.Definition {
		return modelskill.Definition{ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "maxHp", Value: float64(hp)},
		}}
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: expertiseSkillID, Level: 1},
		passive(riggedItemSkillID, riggedItemSkillHP),
		passive(riggedEnchantSkillID, riggedEnchantSkillHP),
		passive(gameservertest.AugmentRedSkillID, riggedAugmentSkillHP),
	}), gamesql.NewCharacterSkillStore(db))
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithItemTemplates(items),
		gameservertest.WithSkills(skills),
		gameservertest.WithAugmentations(gameservertest.AugmentationTable(t), nil, nil),
		gameservertest.WithDataReloads(network.DataReloads{Items: func() error {
			items.Replace(riggedWeaponTable(false))
			return nil
		}}),
	)
	// The rigged weapon is D grade: without Expertise none of its skills
	// would be granted.
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), gmID, 0, expertiseSkillID, 1); err != nil {
		t.Fatalf("grant Expertise: %v", err)
	}
	weapon := srv.GiveItem(t, gmID, riggedWeaponID, 1)
	row := itemRow(t, srv, gmID, weapon)
	row.EnchantLevel = item.Enchant4SkillLevel
	if err := srv.Items.Update(context.Background(), row); err != nil {
		t.Fatalf("enchant the weapon: %v", err)
	}
	aug := item.Augmentation{Attributes: riggedAugmentation, SkillID: gameservertest.AugmentRedSkillID, SkillLevel: 1}
	if err := gamesql.NewAugmentationStore(srv.DB).Create(context.Background(), weapon, aug); err != nil {
		t.Fatalf("augment the weapon: %v", err)
	}
	gm := srv.Client
	bare := lastMaxHP(t, enterWorldBurst(t, gm))

	on := exchange(t, gm, encodeUseItem(weapon))
	brought := int32(riggedWeaponHP + riggedItemSkillHP + riggedEnchantSkillHP + riggedAugmentStatHP + riggedAugmentSkillHP)
	if got := lastMaxHP(t, on); got != bare+brought {
		t.Fatalf("max HP wearing the weapon = %d, want %d", got, bare+brought)
	}
	list := lastFrameOf(t, on, serverpackets.OpcodeSkillList)
	for _, id := range []int32{riggedItemSkillID, riggedEnchantSkillID, gameservertest.AugmentRedSkillID} {
		if !skillListHas(list, id) {
			t.Fatalf("SkillList wearing the weapon lacks skill %d", id)
		}
	}

	assertTexts(t, exchange(t, gm, encodeBuildCmd("reload item")), "Items' templates have been reloaded.")
	off := exchange(t, gm, encodeUnequip(int32(item.SlotRHand)))
	if got := lastMaxHP(t, off); got != bare {
		t.Fatalf("max HP after taking the reloaded-away weapon off = %d, want %d", got, bare)
	}
	list = lastFrameOf(t, off, serverpackets.OpcodeSkillList)
	for _, id := range []int32{riggedItemSkillID, riggedEnchantSkillID, gameservertest.AugmentRedSkillID} {
		if skillListHas(list, id) {
			t.Fatalf("SkillList after taking the reloaded-away weapon off still lists skill %d", id)
		}
	}
	if got := srv.PlayerMaxHP(t, gmID); got != int(bare) {
		t.Fatalf("live max HP after taking the reloaded-away weapon off = %d, want %d", got, bare)
	}
}

// mithrilSetSkillsHP is the max HP bootArmorSetAdmin's complete set adds:
// its set skill's 10 and the common set skill's 1.
const mithrilSetSkillsHP = 10 + 1

// TestReloadedAwayWornSetPieceUnequipTakesOffTheSetSkill wears the Mithril
// set, reloads the item table without one of its pieces and takes a piece
// off: the set skills leave the skill list and their max HP goes, whether the
// piece coming off is the reloaded-away chest or the legs worn with it.
func TestReloadedAwayWornSetPieceUnequipTakesOffTheSetSkill(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		dropped int32
		slot    item.Slot
	}{
		{"reloaded-away chest off", mithrilChestID, item.SlotChest},
		{"legs off under a reloaded-away chest", mithrilChestID, item.SlotLegs},
		{"reloaded-away legs off", mithrilLegsID, item.SlotLegs},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, gmID := bootArmorSetAdminReloading(t, nil, tc.dropped)
			gm := srv.Client
			before := srv.PlayerMaxHP(t, gmID)
			assertTexts(t, exchange(t, gm, encodeBuildCmd("reload item")), "Items' templates have been reloaded.")

			off := exchange(t, gm, encodeUnequip(int32(tc.slot)))
			list := lastFrameOf(t, off, serverpackets.OpcodeSkillList)
			for _, id := range []int32{mithrilSetSkillID, armorSetCommonID} {
				if skillListHas(list, id) {
					t.Fatalf("SkillList after the piece came off still lists set skill %d", id)
				}
			}
			if got, want := srv.PlayerMaxHP(t, gmID), before-mithrilSetSkillsHP; got != want {
				t.Fatalf("max HP after the piece came off = %d, want %d", got, want)
			}
		})
	}
}

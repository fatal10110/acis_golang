package items

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// An augmented weapon above the wearer's Expertise. Equipping it attaches
// the weapon's own stat functions first, and reporting them turns the grade
// penalty on while the weapon is already worn: the penalty takes the
// weapon's augmentation and skills away, none being on yet
// (Player.refreshExpertisePenalty -> ItemPassiveSkillsListener.onUnequip,
// whose AugmentationStatBoni.removeBonus is a no-op while inactive). The
// equip then applies the augmentation ahead of its grade check
// (ItemPassiveSkillsListener.onEquip), so it counts once. Gaining Expertise
// while it is worn applies it again, which the active flag keeps to once
// (AugmentationStatBoni.applyBonus); losing Expertise takes it off.

// The rigged weapon: a copy of the shared D-grade sword adding max HP
// itself, augmented with the solo maxHp option of the first color block
// under the red skill option, whose skill adds max HP too.
const (
	gradeWeaponID       int32 = 65021
	gradeWeaponHP             = 100
	gradeAugmentation   int32 = 14563<<16 | 3
	gradeAugmentStatHP        = 30
	gradeAugmentSkillHP       = 20
	gradeAugmentHP            = gradeAugmentStatHP + gradeAugmentSkillHP
)

// bootAugmentedGradeWeapon boots a level 19 character without Expertise
// whose class grants it at level 20, holding the augmented rigged weapon.
func bootAugmentedGradeWeapon(t *testing.T) (*gameservertest.Server, int32, int32) {
	t.Helper()
	sword, ok := gameservertest.ItemTemplates().Get(30)
	if !ok || sword.Crystal != item.CrystalD {
		t.Fatal("shared catalog lacks the D-grade sword 30")
	}
	rigged := *sword
	rigged.ID = gradeWeaponID
	rigged.Modifiers = []item.StatModifier{{Op: item.FuncAdd, Stat: "maxHp", Value: gradeWeaponHP}}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: expertiseSkillID, Level: 1},
		{
			ID: modelskill.ID(gameservertest.AugmentRedSkillID), Level: 1, Activation: modelskill.ActivationPassive,
			Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: gradeAugmentSkillHP}},
		},
	}), gamesql.NewCharacterSkillStore(db))
	tmpl := gameservertest.ClassTemplate()
	tmpl.Skills = append(tmpl.Skills, player.SkillGrant{SkillID: expertiseSkillID, Level: 1, MinLevel: 20})
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), &rigged))),
		gameservertest.WithSkills(skills),
		gameservertest.WithAugmentations(gameservertest.AugmentationTable(t), nil, nil),
		gameservertest.WithCharacter("Wielder", 19, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(tmpl),
	)
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, gradeWeaponID, 1)
	aug := item.Augmentation{Attributes: gradeAugmentation, SkillID: gameservertest.AugmentRedSkillID, SkillLevel: 1}
	if err := gamesql.NewAugmentationStore(srv.DB).Create(context.Background(), weapon, aug); err != nil {
		t.Fatalf("augment the weapon: %v", err)
	}
	startInWorld(t, srv.Client)
	return srv, objID, weapon
}

// lastSkillListHas reports whether the last SkillList among frames lists id.
func lastSkillListHas(t *testing.T, frames [][]byte, id int32) bool {
	t.Helper()
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i][0] == serverpackets.OpcodeSkillList {
			return skillListHas(t, frames[i], id)
		}
	}
	t.Fatalf("frames %x: no SkillList", testsupport.FrameOpcodes(frames))
	return false
}

// assertMaxHP checks the live max HP and the last UserInfo's.
func assertMaxHP(t *testing.T, srv *gameservertest.Server, objID int32, frames [][]byte, want int, what string) {
	t.Helper()
	if got := srv.PlayerMaxHP(t, objID); got != want {
		t.Fatalf("max HP %s = %d, want %d", what, got, want)
	}
	if got := lastUserInfoMaxHP(t, frames); int(got) != want {
		t.Fatalf("UserInfo MaxHP %s = %d, want %d", what, got, want)
	}
}

// TestAugmentedWeaponAboveExpertiseCountsAugmentationOnce: wearing the
// augmented D-grade weapon without Expertise adds the weapon's own max HP and
// its augmentation's once, the augmentation skill listed; taking it off
// returns max HP to bare.
func TestAugmentedWeaponAboveExpertiseCountsAugmentationOnce(t *testing.T) {
	t.Parallel()
	srv, objID, weapon := bootAugmentedGradeWeapon(t)
	c := srv.Client
	bare := srv.PlayerMaxHP(t, objID)

	c.Send(encodeUseItem(weapon, false))
	on := collectUntilQuiet(t, c)
	if etc := etcStatusFrames(on); len(etc) != 1 || !etcStatusGradePenalty(t, etc[0]) {
		t.Fatalf("equip frames %x: want one EtcStatusUpdate showing the grade penalty", testsupport.FrameOpcodes(on))
	}
	assertMaxHP(t, srv, objID, on, bare+gradeWeaponHP+gradeAugmentHP, "wearing the weapon above Expertise")
	if !lastSkillListHas(t, on, gameservertest.AugmentRedSkillID) {
		t.Fatal("last SkillList wearing the weapon above Expertise lacks the augmentation skill")
	}

	c.Send(encodeRequestUnEquipItem(int32(item.SlotRHand)))
	off := collectUntilQuiet(t, c)
	assertMaxHP(t, srv, objID, off, bare, "after taking the weapon off")
	if lastSkillListHas(t, off, gameservertest.AugmentRedSkillID) {
		t.Fatal("last SkillList after taking the weapon off lists the augmentation skill")
	}
}

// TestAugmentedWeaponFollowsExpertiseWhileWorn: with the augmented weapon
// worn, reaching level 20 grants Expertise and the augmentation still counts
// once; dropping back to 19 loses Expertise and the augmentation with it;
// regaining it brings the augmentation back once, and taking the weapon off
// takes off exactly the weapon and its augmentation.
func TestAugmentedWeaponFollowsExpertiseWhileWorn(t *testing.T) {
	t.Parallel()
	srv, objID, weapon := bootAugmentedGradeWeapon(t)
	c := srv.Client
	bare19 := srv.PlayerMaxHP(t, objID)
	c.Send(encodeUseItem(weapon, false))
	drainUntilQuiet(t, c)
	if got, want := srv.PlayerMaxHP(t, objID), bare19+gradeWeaponHP+gradeAugmentHP; got != want {
		t.Fatalf("max HP wearing the weapon above Expertise = %d, want %d", got, want)
	}

	srv.AddPlayerLevel(t, objID, 1)
	up := collectUntilQuiet(t, c)
	if etc := etcStatusFrames(up); len(etc) != 1 || etcStatusGradePenalty(t, etc[0]) {
		t.Fatalf("level-up frames %x: want one EtcStatusUpdate without the grade penalty", testsupport.FrameOpcodes(up))
	}
	worn20 := srv.PlayerMaxHP(t, objID)
	if !lastSkillListHas(t, up, gameservertest.AugmentRedSkillID) {
		t.Fatal("last SkillList after gaining Expertise lacks the augmentation skill")
	}

	srv.AddPlayerLevel(t, objID, -1)
	down := collectUntilQuiet(t, c)
	if etc := etcStatusFrames(down); len(etc) != 1 || !etcStatusGradePenalty(t, etc[0]) {
		t.Fatalf("level-down frames %x: want one EtcStatusUpdate showing the grade penalty", testsupport.FrameOpcodes(down))
	}
	assertMaxHP(t, srv, objID, down, bare19+gradeWeaponHP, "after losing Expertise")
	if lastSkillListHas(t, down, gameservertest.AugmentRedSkillID) {
		t.Fatal("last SkillList after losing Expertise lists the augmentation skill")
	}

	srv.AddPlayerLevel(t, objID, 1)
	again := collectUntilQuiet(t, c)
	assertMaxHP(t, srv, objID, again, worn20, "after regaining Expertise")
	c.Send(encodeRequestUnEquipItem(int32(item.SlotRHand)))
	off := collectUntilQuiet(t, c)
	assertMaxHP(t, srv, objID, off, worn20-gradeWeaponHP-gradeAugmentHP, "after taking the weapon off at level 20")
}

package items

import (
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// A D-grade weapon carrying an item skill, worn by a character whose
// Expertise comes and goes with its level. Owner decision (#4428): when the
// grade penalty turns on while the weapon stays worn, the weapon's item
// skills are withdrawn with it, which is stricter than the reference
// (Player.refreshExpertisePenalty -> ItemPassiveSkillsListener.onUnequip
// keeps every item_skill entry whose item is still in the paperdoll, and the
// weapon itself always is). The unit pin is
// TestRefreshEquippedItemStatsRevokesNonPassiveAttachedSkillWhenGateIsLost.
const gradeSkillWeaponID int32 = 65022

const (
	gradeItemSkillID = 3098
	gradeItemSkillHP = 15
)

// bootItemSkillGradeWeapon boots a level 19 character without Expertise
// (the class grants it at level 20) holding a D-grade weapon whose item
// skill adds max HP. The weapon is not worn on return.
func bootItemSkillGradeWeapon(t *testing.T) (*gameservertest.Server, int32, int32) {
	t.Helper()
	sword, ok := gameservertest.ItemTemplates().Get(30)
	if !ok || sword.Crystal != item.CrystalD {
		t.Fatal("shared catalog lacks the D-grade sword 30")
	}
	weapon := *sword
	weapon.ID = gradeSkillWeaponID
	weapon.AttachedSkills = []item.SkillRef{{ID: gradeItemSkillID, Level: 1}}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: expertiseSkillID, Level: 1},
		{
			ID: gradeItemSkillID, Level: 1, Activation: modelskill.ActivationPassive,
			Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: gradeItemSkillHP}},
		},
	}), gamesql.NewCharacterSkillStore(db))
	tmpl := gameservertest.ClassTemplate()
	tmpl.Skills = append(tmpl.Skills, player.SkillGrant{SkillID: expertiseSkillID, Level: 1, MinLevel: 20})
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), &weapon))),
		gameservertest.WithSkills(skills),
		gameservertest.WithCharacter("Wielder", 19, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(tmpl),
	)
	objID := srv.SoleObjectID(t)
	obj := srv.GiveItem(t, objID, gradeSkillWeaponID, 1)
	startInWorld(t, srv.Client)
	return srv, objID, obj
}

// TestGradePenaltyWithdrawsWornWeaponItemSkill: the weapon's item skill stays
// off while the wearer lacks Expertise, comes with Expertise at level 20,
// and leaves again, SkillList and max HP both, when the level drops back to
// 19 with the weapon still worn.
func TestGradePenaltyWithdrawsWornWeaponItemSkill(t *testing.T) {
	t.Parallel()
	srv, objID, weapon := bootItemSkillGradeWeapon(t)
	c := srv.Client
	bare := srv.PlayerMaxHP(t, objID)

	c.Send(encodeUseItem(weapon, false))
	worn := collectUntilQuiet(t, c)
	if lastSkillListHas(t, worn, gradeItemSkillID) {
		t.Fatal("last SkillList wearing the weapon above Expertise lists the item skill")
	}
	assertMaxHP(t, srv, objID, worn, bare, "wearing the weapon above Expertise")

	srv.AddPlayerLevel(t, objID, 1)
	up := collectUntilQuiet(t, c)
	if etc := etcStatusFrames(up); len(etc) != 1 || etcStatusGradePenalty(t, etc[0]) {
		t.Fatalf("level-up frames %x: want one EtcStatusUpdate without the grade penalty", testsupport.FrameOpcodes(up))
	}
	if !lastSkillListHas(t, up, gradeItemSkillID) {
		t.Fatal("last SkillList after gaining Expertise lacks the item skill")
	}
	if got := srv.PlayerSkillLevel(t, objID, gradeItemSkillID); got != 1 {
		t.Fatalf("item skill level at 20 = %d, want 1", got)
	}
	withSkill := srv.PlayerMaxHP(t, objID)

	srv.AddPlayerLevel(t, objID, -1)
	down := collectUntilQuiet(t, c)
	if etc := etcStatusFrames(down); len(etc) != 1 || !etcStatusGradePenalty(t, etc[0]) {
		t.Fatalf("level-down frames %x: want one EtcStatusUpdate showing the grade penalty", testsupport.FrameOpcodes(down))
	}
	if lastSkillListHas(t, down, gradeItemSkillID) {
		t.Fatal("last SkillList after losing Expertise still lists the item skill")
	}
	if got := srv.PlayerSkillLevel(t, objID, gradeItemSkillID); got != 0 {
		t.Fatalf("item skill level after losing Expertise = %d, want 0", got)
	}
	if got := srv.PlayerMaxHP(t, objID); got >= withSkill {
		t.Fatalf("max HP after losing Expertise = %d, want below %d with the item skill", got, withSkill)
	}
}

// TestAugmentedOverGradeWeaponSurvivesRelog: an augmented weapon worn above
// the wearer's Expertise keeps its augmentation across a relog: max HP and
// SkillList at login carry it once, and taking the weapon off removes it.
func TestAugmentedOverGradeWeaponSurvivesRelog(t *testing.T) {
	t.Parallel()
	srv, objID, weapon := bootAugmentedGradeWeapon(t)
	bare := srv.PlayerMaxHP(t, objID)

	srv.Client.Send(encodeUseItem(weapon, false))
	drainUntilQuiet(t, srv.Client)
	want := bare + gradeWeaponHP + gradeAugmentHP
	if got := srv.PlayerMaxHP(t, objID); got != want {
		t.Fatalf("max HP wearing the weapon above Expertise = %d, want %d", got, want)
	}

	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	for srv.Client.Read()[0] != serverpackets.OpcodeLeaveWorld {
	}
	srv.Client.ExpectClosed()
	srv.FlushPersistence(t)

	relogin := srv.DialClient(t, "player1", 1)
	frames := startInWorld(t, relogin)
	if !lastSkillListHas(t, frames, gameservertest.AugmentRedSkillID) {
		t.Fatal("login SkillList lacks the augmentation skill")
	}
	worn := srv.PlayerMaxHP(t, objID)
	if got := lastUserInfoMaxHP(t, frames); int(got) != worn {
		t.Fatalf("login UserInfo MaxHP = %d, live max HP = %d", got, worn)
	}

	// The restored character's bare max HP is its own (the fixture's fresh
	// character differs), so the worn bonus is measured against it.
	relogin.Send(encodeRequestUnEquipItem(int32(item.SlotRHand)))
	off := collectUntilQuiet(t, relogin)
	assertMaxHP(t, srv, objID, off, worn-gradeWeaponHP-gradeAugmentHP, "after taking the relogged weapon off")
	if lastSkillListHas(t, off, gameservertest.AugmentRedSkillID) {
		t.Fatal("last SkillList after taking the relogged weapon off lists the augmentation skill")
	}
}

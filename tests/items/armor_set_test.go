package items

import (
	"context"
	"path/filepath"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The shipped Mithril Heavy Set (armorSets.xml): chest 58, legs 59, head 47,
// shield 628 (Hoplon), set skill 3502, shield skill 3543, +6 skill 3611, all
// D grade. The scenarios give each granted skill a distinct maxHp bonus so
// every grant and removal shows in the live MaxHP.
const (
	mithrilChestID    int32 = 58
	mithrilLegsID     int32 = 59
	mithrilHeadID     int32 = 47
	mithrilShieldID   int32 = 628
	armorSetCommonID        = 3006
	mithrilSetSkillID       = 3502
	mithrilShieldSkID       = 3543
	mithrilEnchant6ID       = 3611
	armorSetCommonHP        = 1
	mithrilSetHP            = 10
	mithrilShieldHP         = 100
	mithrilEnchant6HP       = 1000
	armorScrollD      int32 = 956
	blessedArmorD     int32 = 6576
)

var mithrilPieceSlots = map[int32]int{
	mithrilChestID:  itemcontainer.Chest,
	mithrilLegsID:   itemcontainer.Legs,
	mithrilHeadID:   itemcontainer.Head,
	mithrilShieldID: itemcontainer.LHand,
}

// armorSetRig is a booted character holding the Mithril Heavy Set.
type armorSetRig struct {
	srv     *gameservertest.Server
	objID   int32
	objects map[int32]int32 // object id by template id
}

// bootArmorSet boots a character that knows Expertise against the shipped
// armor set table, holding the Mithril set's pieces and shield at the given
// enchant levels plus scroll, when not zero. Pieces listed in worn start on
// the paperdoll at login.
func bootArmorSet(t *testing.T, roll float64, enchant map[int32]int, worn []int32, scroll int32) armorSetRig {
	t.Helper()
	datapack.Require(t)
	_, shipped := shippedData()
	dir, _ := datapack.Find()
	sets, err := xmldata.LoadArmorSets(filepath.Join(dir, "data", "xml", "armorSets.xml"))
	if err != nil {
		t.Fatalf("load armor sets: %v", err)
	}
	ids := []int32{mithrilChestID, mithrilLegsID, mithrilHeadID, mithrilShieldID}
	templates := gameservertest.ItemTemplates().All()
	for _, id := range append(ids, armorScrollD, blessedArmorD) {
		tmpl, ok := shipped.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	passive := func(id, hp int) modelskill.Definition {
		return modelskill.Definition{ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "maxHp", Value: float64(hp)},
		}}
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: expertiseSkillID, Level: 1},
		passive(armorSetCommonID, armorSetCommonHP),
		passive(mithrilSetSkillID, mithrilSetHP),
		passive(mithrilShieldSkID, mithrilShieldHP),
		passive(mithrilEnchant6ID, mithrilEnchant6HP),
	}), gamesql.NewCharacterSkillStore(db))
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithSkills(skills),
		gameservertest.WithArmorSets(sets),
		gameservertest.WithEnchantRoll(func() float64 { return roll }),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, expertiseSkillID, 1); err != nil {
		t.Fatalf("grant Expertise: %v", err)
	}
	rig := armorSetRig{srv: srv, objID: objID, objects: map[int32]int32{}}
	for _, id := range ids {
		rig.objects[id] = srv.GiveItem(t, objID, id, 1)
		inst := mustFindItem(t, srv, objID, rig.objects[id])
		inst.EnchantLevel = enchant[id]
		for _, w := range worn {
			if w == id {
				inst.Location, inst.LocationData = item.LocationPaperdoll, mithrilPieceSlots[id]
			}
		}
		if err := srv.Items.Update(context.Background(), inst); err != nil {
			t.Fatalf("seed item %d: %v", id, err)
		}
	}
	if scroll != 0 {
		rig.objects[scroll] = srv.GiveItem(t, objID, scroll, 1)
	}
	return rig
}

// toggle uses the item of template id from the item window, equipping or
// unequipping it, and returns the frames the server answered with.
func (r armorSetRig) toggle(t *testing.T, id int32) [][]byte {
	t.Helper()
	r.srv.Client.Send(encodeUseItem(r.objects[id], false))
	frames := collectUntilQuiet(t, r.srv.Client)
	r.srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, r.srv.Client)
	return frames
}

// skillLists returns the SkillList frames among frames, in order.
func skillLists(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSkillList {
			out = append(out, f)
		}
	}
	return out
}

// assertSkillListIDs requires frame to list every id in has and none in
// lacks.
func assertSkillListIDs(t *testing.T, label string, frame []byte, has, lacks []int32) {
	t.Helper()
	for _, id := range has {
		if !skillListHas(t, frame, id) {
			t.Fatalf("%s: SkillList lacks skill %d", label, id)
		}
	}
	for _, id := range lacks {
		if skillListHas(t, frame, id) {
			t.Fatalf("%s: SkillList lists skill %d", label, id)
		}
	}
}

// TestArmorSetEquipGrantsSetSkills pins ArmorSetListener: the piece that
// completes a worn set grants the common and set skills with one SkillList,
// then the shield skill with a second while the set's shield is held;
// taking a set piece off removes all of them with one SkillList, and taking
// the shield off removes the shield skill alone. Equipping a piece of an
// incomplete set, or the shield without a set, sends no SkillList.
func TestArmorSetEquipGrantsSetSkills(t *testing.T) {
	t.Parallel()
	rig := bootArmorSet(t, 0, nil, nil, 0)
	startInWorld(t, rig.srv.Client)
	baseHP := rig.srv.PlayerMaxHP(t, rig.objID)
	setHP := armorSetCommonHP + mithrilSetHP

	for _, id := range []int32{mithrilShieldID, mithrilChestID, mithrilLegsID} {
		if lists := skillLists(rig.toggle(t, id)); len(lists) != 0 {
			t.Fatalf("equipping %d before the set is complete sent %d SkillList(s)", id, len(lists))
		}
	}
	if got := rig.srv.PlayerMaxHP(t, rig.objID); got != baseHP {
		t.Fatalf("MaxHP with an incomplete set = %d, want %d", got, baseHP)
	}

	lists := skillLists(rig.toggle(t, mithrilHeadID))
	if len(lists) != 2 {
		t.Fatalf("completing the set with the shield held sent %d SkillLists, want 2", len(lists))
	}
	assertSkillListIDs(t, "set grant", lists[0], []int32{armorSetCommonID, mithrilSetSkillID}, []int32{mithrilShieldSkID, mithrilEnchant6ID})
	assertSkillListIDs(t, "shield grant", lists[1], []int32{armorSetCommonID, mithrilSetSkillID, mithrilShieldSkID}, []int32{mithrilEnchant6ID})
	if got, want := rig.srv.PlayerMaxHP(t, rig.objID), baseHP+setHP+mithrilShieldHP; got != want {
		t.Fatalf("MaxHP with the full set and shield = %d, want %d", got, want)
	}

	lists = skillLists(rig.toggle(t, mithrilLegsID))
	if len(lists) != 1 {
		t.Fatalf("taking the legs off sent %d SkillLists, want 1", len(lists))
	}
	assertSkillListIDs(t, "set piece removal", lists[0], nil, []int32{armorSetCommonID, mithrilSetSkillID, mithrilShieldSkID})
	if got := rig.srv.PlayerMaxHP(t, rig.objID); got != baseHP {
		t.Fatalf("MaxHP after taking the legs off = %d, want %d", got, baseHP)
	}

	if lists = skillLists(rig.toggle(t, mithrilLegsID)); len(lists) != 2 {
		t.Fatalf("putting the legs back on sent %d SkillLists, want 2", len(lists))
	}
	lists = skillLists(rig.toggle(t, mithrilShieldID))
	if len(lists) != 1 {
		t.Fatalf("taking the shield off sent %d SkillLists, want 1", len(lists))
	}
	assertSkillListIDs(t, "shield removal", lists[0], []int32{armorSetCommonID, mithrilSetSkillID}, []int32{mithrilShieldSkID})
	if got, want := rig.srv.PlayerMaxHP(t, rig.objID), baseHP+setHP; got != want {
		t.Fatalf("MaxHP with the set and no shield = %d, want %d", got, want)
	}

	lists = skillLists(rig.toggle(t, mithrilShieldID))
	if len(lists) != 1 {
		t.Fatalf("taking the shield back with the set complete sent %d SkillLists, want 1", len(lists))
	}
	assertSkillListIDs(t, "shield regrant", lists[0], []int32{mithrilShieldSkID}, nil)
}

// TestArmorSetEnchant6Skill pins the +6 set skill: equipping the piece that
// completes a set worn at +6 throughout grants it with its own SkillList
// after the set's, and taking the chest off removes every set skill; a set
// worn at login is restored with its skills; a scroll success taking the last worn piece to
// +6 grants the +6 skill with a SkillList between the success message and
// EnchantResult, and one that leaves another worn piece below +6 sends no
// SkillList and grants nothing; a failure on a worn piece at +6 or higher removes it with
// a SkillList ahead of the failure's own message.
func TestArmorSetEnchant6Skill(t *testing.T) {
	t.Parallel()
	pieces := []int32{mithrilChestID, mithrilLegsID, mithrilHeadID}
	t.Run("success to +6 grants it", func(t *testing.T) {
		t.Parallel()
		rig := bootArmorSet(t, 0, map[int32]int{mithrilChestID: 6, mithrilLegsID: 6, mithrilHeadID: 5}, pieces, armorScrollD)
		c := rig.srv.Client
		var login []byte
		for _, f := range startInWorld(t, c) {
			if f[0] == serverpackets.OpcodeSkillList {
				login = f
			}
		}
		if login == nil {
			t.Fatal("EnterWorld burst has no SkillList")
		}
		assertSkillListIDs(t, "login", login, []int32{armorSetCommonID, mithrilSetSkillID}, []int32{mithrilEnchant6ID, mithrilShieldSkID})
		before := rig.srv.PlayerMaxHP(t, rig.objID)

		openEnchantSelection(t, c, rig.objects[armorScrollD], armorScrollD)
		c.Send(encodeRequestEnchantItem(rig.objects[mithrilHeadID]))
		frames := collectUntilQuiet(t, c)
		msg := frameIndex(t, frames, 0, serverpackets.OpcodeSystemMessage, serverpackets.SystemMessageS1S2SuccessfullyEnchanted)
		skillList := frameIndex(t, frames, 0, serverpackets.OpcodeSkillList, 0)
		result := frameIndex(t, frames, 0, serverpackets.OpcodeEnchantResult, 0)
		if msg < 0 || skillList < msg || result < skillList {
			t.Fatalf("frames %x: want the success message, then SkillList, then EnchantResult", opcodes(frames))
		}
		assertSkillListIDs(t, "+6 grant", frames[skillList], []int32{mithrilEnchant6ID, mithrilSetSkillID}, nil)
		assertEnchantResult(t, frames[result], serverpackets.EnchantResultSuccess)
		if got, want := rig.srv.PlayerMaxHP(t, rig.objID), before+mithrilEnchant6HP; got != want {
			t.Fatalf("MaxHP after the +6 grant = %d, want %d", got, want)
		}
	})

	t.Run("success to +6 with another piece below +6 grants nothing", func(t *testing.T) {
		t.Parallel()
		rig := bootArmorSet(t, 0, map[int32]int{mithrilChestID: 6, mithrilLegsID: 5, mithrilHeadID: 5}, pieces, armorScrollD)
		c := rig.srv.Client
		startInWorld(t, c)
		before := rig.srv.PlayerMaxHP(t, rig.objID)

		openEnchantSelection(t, c, rig.objects[armorScrollD], armorScrollD)
		c.Send(encodeRequestEnchantItem(rig.objects[mithrilLegsID]))
		frames := collectUntilQuiet(t, c)
		msg := frameIndex(t, frames, 0, serverpackets.OpcodeSystemMessage, serverpackets.SystemMessageS1S2SuccessfullyEnchanted)
		result := frameIndex(t, frames, 0, serverpackets.OpcodeEnchantResult, 0)
		if msg < 0 || result < msg {
			t.Fatalf("frames %x: want the success message, then EnchantResult", opcodes(frames))
		}
		if lists := skillLists(frames[msg:result]); len(lists) != 0 {
			t.Fatalf("frames %x: legs to +6 with the head at +5 sent %d SkillList(s), want 0", opcodes(frames), len(lists))
		}
		assertEnchantResult(t, frames[result], serverpackets.EnchantResultSuccess)
		if got := rig.srv.PlayerMaxHP(t, rig.objID); got != before {
			t.Fatalf("MaxHP after the legs reached +6 = %d, want %d", got, before)
		}
	})

	t.Run("equipping the last +6 piece grants it", func(t *testing.T) {
		t.Parallel()
		rig := bootArmorSet(t, 0, map[int32]int{mithrilChestID: 6, mithrilLegsID: 6, mithrilHeadID: 6}, nil, 0)
		startInWorld(t, rig.srv.Client)
		baseHP := rig.srv.PlayerMaxHP(t, rig.objID)
		rig.toggle(t, mithrilChestID)
		rig.toggle(t, mithrilLegsID)
		lists := skillLists(rig.toggle(t, mithrilHeadID))
		if len(lists) != 2 {
			t.Fatalf("completing a +6 set sent %d SkillLists, want 2", len(lists))
		}
		assertSkillListIDs(t, "set grant", lists[0], []int32{armorSetCommonID, mithrilSetSkillID}, []int32{mithrilEnchant6ID})
		assertSkillListIDs(t, "+6 grant", lists[1], []int32{mithrilEnchant6ID}, nil)
		if got, want := rig.srv.PlayerMaxHP(t, rig.objID), baseHP+armorSetCommonHP+mithrilSetHP+mithrilEnchant6HP; got != want {
			t.Fatalf("MaxHP with the +6 set = %d, want %d", got, want)
		}

		lists = skillLists(rig.toggle(t, mithrilChestID))
		if len(lists) != 1 {
			t.Fatalf("taking the chest off sent %d SkillLists, want 1", len(lists))
		}
		assertSkillListIDs(t, "chest removal", lists[0], nil, []int32{armorSetCommonID, mithrilSetSkillID, mithrilEnchant6ID})
		if got := rig.srv.PlayerMaxHP(t, rig.objID); got != baseHP {
			t.Fatalf("MaxHP after taking the chest off = %d, want %d", got, baseHP)
		}
	})

	t.Run("blessed failure at +6 revokes it", func(t *testing.T) {
		t.Parallel()
		rig := bootArmorSet(t, 0.99, map[int32]int{mithrilChestID: 6, mithrilLegsID: 6, mithrilHeadID: 6}, pieces, blessedArmorD)
		c := rig.srv.Client
		var login []byte
		for _, f := range startInWorld(t, c) {
			if f[0] == serverpackets.OpcodeSkillList {
				login = f
			}
		}
		if login == nil {
			t.Fatal("EnterWorld burst has no SkillList")
		}
		assertSkillListIDs(t, "login", login, []int32{armorSetCommonID, mithrilSetSkillID, mithrilEnchant6ID}, nil)
		before := rig.srv.PlayerMaxHP(t, rig.objID)

		openEnchantSelection(t, c, rig.objects[blessedArmorD], blessedArmorD)
		c.Send(encodeRequestEnchantItem(rig.objects[mithrilHeadID]))
		frames := collectUntilQuiet(t, c)
		skillList := frameIndex(t, frames, 0, serverpackets.OpcodeSkillList, 0)
		msg := frameIndex(t, frames, 0, serverpackets.OpcodeSystemMessage, serverpackets.SystemMessageBlessedEnchantFailed)
		result := frameIndex(t, frames, 0, serverpackets.OpcodeEnchantResult, 0)
		if skillList < 0 || msg < skillList || result < msg {
			t.Fatalf("frames %x: want SkillList, then the blessed failure message, then EnchantResult", opcodes(frames))
		}
		assertSkillListIDs(t, "+6 removal", frames[skillList], []int32{armorSetCommonID, mithrilSetSkillID}, []int32{mithrilEnchant6ID})
		assertEnchantResult(t, frames[result], serverpackets.EnchantResultUnsuccess)
		if got, want := rig.srv.PlayerMaxHP(t, rig.objID), before-mithrilEnchant6HP; got != want {
			t.Fatalf("MaxHP after the +6 removal = %d, want %d", got, want)
		}
	})
}

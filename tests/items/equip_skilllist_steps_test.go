package items

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	sqltest "github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// An equip change that moves several paperdoll slots runs the listeners of
// each slot in turn (Inventory.setPaperdollItem, Inventory.java:381-433),
// one slot at a time, so every listener sees the paperdoll as it stood at
// its own step and sends its own SkillList (ArmorSetListener,
// ItemPassiveSkillsListener).

// cottonRobeID is a shipped no-grade full-body armor (bodypart fullarmor)
// that is no set's chest and grants no skill.
const cottonRobeID int32 = 427

// skillListFlags returns the formal wear flag of each SkillList among
// frames, in order: 1 when its entries are greyed out.
func skillListFlags(t *testing.T, frames [][]byte) []uint8 {
	t.Helper()
	var out []uint8
	for _, f := range skillLists(frames) {
		entries := readSkillList(t, f)
		if len(entries) == 0 {
			t.Fatal("SkillList is empty")
		}
		out = append(out, entries[0].disabled)
		for _, e := range entries {
			if e.disabled != entries[0].disabled {
				t.Fatalf("SkillList mixes greyed and lit entries: %+v", entries)
			}
		}
	}
	return out
}

// statUserInfosBetweenSkillLists requires at least one UserInfo between
// each pair of consecutive SkillLists among the first lists: every set piece
// taken off carries its own stat funcs, whose removal refreshes the stats
// at that step, ahead of that step's SkillList.
func statUserInfosBetweenSkillLists(t *testing.T, frames [][]byte, lists int) {
	t.Helper()
	seen, userInfo := 0, false
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeUserInfo:
			userInfo = true
		case serverpackets.OpcodeSkillList:
			if seen > 0 && seen < lists && !userInfo {
				t.Fatalf("frames %x: SkillList %d follows SkillList %d with no stat UserInfo between them", opcodes(frames), seen+1, seen)
			}
			seen++
			userInfo = false
		}
	}
}

// TestFormalWearOverSetSendsSkillListPerStep puts formal wear on over the
// worn Mithril set and its shield. The dress clears the legs, left hand,
// right hand, head, feet and gloves before it replaces the chest
// (Inventory.equipItem SLOT_ALLDRESS, Inventory.java:615-622), so the legs,
// the shield and the head each come off against the old set chest, each
// with its own SkillList, then the chest with one more, all lit; only the
// dress's own refresh, once it is worn, greys every skill out.
func TestFormalWearOverSetSendsSkillListPerStep(t *testing.T) {
	t.Parallel()
	worn := []int32{mithrilChestID, mithrilLegsID, mithrilHeadID, mithrilShieldID}
	rig := bootArmorSet(t, 0, nil, worn, 0)
	dress := rig.srv.GiveItem(t, rig.objID, gameservertest.FormalWearID, 1)
	startInWorld(t, rig.srv.Client)

	rig.srv.Client.Send(encodeUseItem(dress, false))
	frames := collectUntilQuiet(t, rig.srv.Client)
	lists := skillLists(frames)
	if len(lists) != 5 {
		t.Fatalf("frames %x: formal wear over the set sent %d SkillLists, want 5 (legs, shield, head, chest, dress)", opcodes(frames), len(lists))
	}
	setSkills := []int32{armorSetCommonID, mithrilSetSkillID, mithrilShieldSkID}
	for _, list := range lists {
		assertSkillListIDs(t, "formal wear step", list, []int32{expertiseSkillID}, setSkills)
	}
	if got, want := skillListFlags(t, frames), []uint8{0, 0, 0, 0, 1}; string(got) != string(want) {
		t.Fatalf("SkillList greyed flags = %v, want %v (only the dress's own refresh greys out)", got, want)
	}
	statUserInfosBetweenSkillLists(t, frames, 4)
}

// TestFullArmorOverSetSendsSkillListPerStep puts a full-body robe on over
// the worn Mithril chest, legs and head. The robe clears the legs before it
// replaces the chest (Inventory.equipItem SLOT_FULL_ARMOR,
// Inventory.java:556-559), so the legs come off against the old set chest
// with their own SkillList, then the chest with one more.
func TestFullArmorOverSetSendsSkillListPerStep(t *testing.T) {
	t.Parallel()
	worn := []int32{mithrilChestID, mithrilLegsID, mithrilHeadID}
	rig := bootArmorSetWith(t, 0, nil, worn, 0, []int32{cottonRobeID})
	startInWorld(t, rig.srv.Client)
	baseHP := rig.srv.PlayerMaxHP(t, rig.objID) - armorSetCommonHP - mithrilSetHP

	frames := rig.toggle(t, cottonRobeID)
	lists := skillLists(frames)
	if len(lists) != 2 {
		t.Fatalf("frames %x: the robe over the set sent %d SkillLists, want 2 (legs, then chest)", opcodes(frames), len(lists))
	}
	for _, list := range lists {
		assertSkillListIDs(t, "full armor step", list, []int32{expertiseSkillID}, []int32{armorSetCommonID, mithrilSetSkillID})
	}
	statUserInfosBetweenSkillLists(t, frames, 2)
	if got := rig.srv.PlayerMaxHP(t, rig.objID); got != baseHP {
		t.Fatalf("MaxHP with the robe on = %d, want %d", got, baseHP)
	}
}

// A second rigged sword: a copy of the fixture sword with its own passive
// item skill.
const riggedSword2ID int32 = 31

const riggedSword2SkillID = 3098

// bootTwoRiggedSwords boots a character that knows Expertise and carries
// the rigged sword twice and the second rigged sword once, and returns the
// server and their object ids in that order.
func bootTwoRiggedSwords(t *testing.T) (*gameservertest.Server, [3]int32) {
	t.Helper()
	var templates []*item.Template
	for _, tmpl := range gameservertest.ItemTemplates().All() {
		if tmpl.ID != riggedSwordID {
			templates = append(templates, tmpl)
			continue
		}
		first, second := *tmpl, *tmpl
		first.AttachedSkills = []item.SkillRef{{ID: riggedSwordSkillID, Level: 1}}
		second.ID, second.Name = riggedSword2ID, "Second Sword"
		second.AttachedSkills = []item.SkillRef{{ID: riggedSword2SkillID, Level: 1}}
		templates = append(templates, &first, &second)
	}
	passive := func(id int) modelskill.Definition {
		return modelskill.Definition{ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "maxHp", Value: riggedSwordPassiveHP},
		}}
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: expertiseSkillID, Level: 1},
		passive(riggedSwordSkillID),
		passive(riggedSword2SkillID),
	}), gamesql.NewCharacterSkillStore(db))
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithSkills(skills),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, expertiseSkillID, 1); err != nil {
		t.Fatalf("grant Expertise: %v", err)
	}
	return srv, [3]int32{
		srv.GiveItem(t, objID, riggedSwordID, 1),
		srv.GiveItem(t, objID, riggedSwordID, 1),
		srv.GiveItem(t, objID, riggedSword2ID, 1),
	}
}

// TestWeaponSwapSendsSkillListPerItem swaps worn weapons that both grant
// an item skill. The right hand's old weapon comes off before the new one
// goes on (Inventory.setPaperdollItem), each step answered by
// ItemPassiveSkillsListener with its own SkillList: the first without the
// old weapon's skill, the second with the new one's. A copy of the worn
// weapon swapped in the same way loses the skill at the first step too,
// since the paperdoll holds no copy of it then, and gets it back at the
// second.
func TestWeaponSwapSendsSkillListPerItem(t *testing.T) {
	t.Parallel()
	srv, swords := bootTwoRiggedSwords(t)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	startInWorld(t, c)
	baseHP := srv.PlayerMaxHP(t, objID)

	use := func(obj int32) [][]byte {
		c.Send(encodeUseItem(obj, false))
		frames := collectUntilQuiet(t, c)
		srv.InventoryUpdates.Tick()
		drainUntilQuiet(t, c)
		return frames
	}
	if lists := skillLists(use(swords[0])); len(lists) != 1 {
		t.Fatalf("equipping the first sword sent %d SkillLists, want 1", len(lists))
	}

	frames := use(swords[2])
	lists := skillLists(frames)
	if len(lists) != 2 {
		t.Fatalf("frames %x: swapping in the second sword sent %d SkillLists, want 2", opcodes(frames), len(lists))
	}
	assertSkillListIDs(t, "old sword off", lists[0], nil, []int32{riggedSwordSkillID, riggedSword2SkillID})
	assertSkillListIDs(t, "new sword on", lists[1], []int32{riggedSword2SkillID}, []int32{riggedSwordSkillID})

	frames = use(swords[0])
	if lists = skillLists(frames); len(lists) != 2 {
		t.Fatalf("frames %x: swapping the first sword back sent %d SkillLists, want 2", opcodes(frames), len(lists))
	}

	frames = use(swords[1])
	lists = skillLists(frames)
	if len(lists) != 2 {
		t.Fatalf("frames %x: swapping in a copy of the worn sword sent %d SkillLists, want 2", opcodes(frames), len(lists))
	}
	assertSkillListIDs(t, "worn copy off", lists[0], nil, []int32{riggedSwordSkillID})
	assertSkillListIDs(t, "other copy on", lists[1], []int32{riggedSwordSkillID}, nil)
	if got, want := srv.PlayerMaxHP(t, objID), baseHP+riggedSwordPassiveHP; got != want {
		t.Fatalf("MaxHP with the copy worn = %d, want %d", got, want)
	}
}

package items

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Shipped equipment the formal wear scenarios put on over the dress: the
// dress itself (bodypart alldress) plus one item per guarded body part.
const (
	formalWearID     int32 = 6408
	leatherHelmetID  int32 = 44 // head
	shortGlovesID    int32 = 48 // gloves
	leatherPantsID   int32 = 29 // legs
	shortSwordID     int32 = 1  // rhand
	leatherShieldID  int32 = 18 // lhand
	formalSkillID          = 248
	formalSkillLevel       = 3
)

// bootFormalWear boots a character that knows one skill and carries the
// shipped formal wear and guarded equipment, and returns the object ids
// keyed by template id.
func bootFormalWear(t *testing.T) (*gameservertest.Server, map[int32]int32) {
	t.Helper()
	datapack.Require(t)
	_, shipped := shippedData()
	ids := []int32{formalWearID, leatherHelmetID, shortGlovesID, leatherPantsID, shortSwordID, leatherShieldID}
	var templates []*item.Template
	for _, tmpl := range gameservertest.ItemTemplates().All() {
		if tmpl.ID != formalWearID {
			templates = append(templates, tmpl)
		}
	}
	for _, id := range ids {
		tmpl, ok := shipped.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, formalSkillID, formalSkillLevel); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	objects := make(map[int32]int32, len(ids))
	for _, id := range ids {
		objects[id] = srv.GiveItem(t, objID, id, 1)
	}
	return srv, objects
}

type skillListEntry struct {
	passive  int32
	level    int32
	id       int32
	disabled uint8
}

func readSkillList(t *testing.T, frame []byte) []skillListEntry {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSkillList, "SkillList")
	r := wire.NewReader(frame[1:])
	n := int(r.ReadInt32())
	entries := make([]skillListEntry, n)
	for i := range entries {
		entries[i] = skillListEntry{passive: r.ReadInt32(), level: r.ReadInt32(), id: r.ReadInt32(), disabled: r.ReadUint8()}
	}
	return entries
}

// assertSkillListDisabled requires the frames to hold exactly one SkillList,
// ahead of the UserInfo refresh, whose every entry (including the seeded
// skill) carries the wanted disabled flag.
func assertSkillListDisabled(t *testing.T, frames [][]byte, want uint8) {
	t.Helper()
	skillList, userInfo := -1, -1
	for i, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSkillList:
			if skillList >= 0 {
				t.Fatalf("second SkillList at frame %d", i)
			}
			skillList = i
		case serverpackets.OpcodeUserInfo:
			userInfo = i
		}
	}
	// A stat func change of the swapped items sends its own UserInfo ahead
	// of the item-skill listener's SkillList; the SkillList still precedes
	// the closing UserInfo refresh.
	if skillList < 0 || userInfo < 0 || skillList > userInfo {
		t.Fatalf("SkillList at frame %d, last UserInfo at %d: want a SkillList before the closing UserInfo", skillList, userInfo)
	}
	entries := readSkillList(t, frames[skillList])
	seeded := false
	for _, e := range entries {
		if e.disabled != want {
			t.Fatalf("SkillList entry %+v disabled = %d, want %d", e, e.disabled, want)
		}
		seeded = seeded || (e.id == formalSkillID && e.level == formalSkillLevel)
	}
	if !seeded {
		t.Fatalf("SkillList %+v lacks the seeded skill %d", entries, formalSkillID)
	}
}

// TestFormalWearGreysSkillList pins the dress's skill window refresh:
// putting formal wear on resends SkillList with every skill greyed out, and
// taking it off resends it with every skill lit again.
func TestFormalWearGreysSkillList(t *testing.T) {
	t.Parallel()
	srv, objects := bootFormalWear(t)
	c := srv.Client
	startInWorld(t, c)

	c.Send(encodeUseItem(objects[formalWearID], false))
	assertSkillListDisabled(t, collectUntilQuiet(t, c), 1)

	c.Send(encodeUseItem(objects[formalWearID], false))
	assertSkillListDisabled(t, collectUntilQuiet(t, c), 0)
}

// TestFormalWearGreysEnterWorldSkillList pins the same flag on the
// EnterWorld SkillList of a character logging in with the dress on.
func TestFormalWearGreysEnterWorldSkillList(t *testing.T) {
	t.Parallel()
	srv, objects := bootFormalWear(t)
	objID := srv.SoleObjectID(t)
	dress := mustFindItem(t, srv, objID, objects[formalWearID])
	dress.Location, dress.LocationData = item.LocationPaperdoll, itemcontainer.Chest
	if err := srv.Items.Update(context.Background(), dress); err != nil {
		t.Fatalf("seed worn dress: %v", err)
	}

	var skillList []byte
	for _, f := range startInWorld(t, srv.Client) {
		if f[0] == serverpackets.OpcodeSkillList {
			skillList = f
		}
	}
	if skillList == nil {
		t.Fatal("EnterWorld burst has no SkillList")
	}
	entries := readSkillList(t, skillList)
	if len(entries) == 0 {
		t.Fatal("EnterWorld SkillList is empty")
	}
	for _, e := range entries {
		if e.disabled != 1 {
			t.Fatalf("EnterWorld SkillList entry %+v not disabled under formal wear", e)
		}
	}
}

// TestFormalWearRefusesArmorPieces pins the dressed equip guard for legs,
// gloves and head: each equip answers
// CANNOT_EQUIP_ITEM_DUE_TO_BAD_CONDITION then the UserInfo refresh, and the
// dress and the refused piece stay where they were.
func TestFormalWearRefusesArmorPieces(t *testing.T) {
	t.Parallel()
	srv, objects := bootFormalWear(t)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	startInWorld(t, c)

	c.Send(encodeUseItem(objects[formalWearID], false))
	drainUntilQuiet(t, c)
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)

	for _, id := range []int32{leatherHelmetID, shortGlovesID, leatherPantsID} {
		c.Send(encodeUseItem(objects[id], false))
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotEquipItemDueToBadCondition)
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeUserInfo, "refusal UserInfo")
		for _, f := range collectUntilQuiet(t, c) {
			if f[0] == serverpackets.OpcodeSkillList || f[0] == serverpackets.OpcodeInventoryUpdate {
				t.Fatalf("refused item %d produced opcode %#x", id, f[0])
			}
		}
	}
	srv.InventoryUpdates.Tick()
	for _, f := range collectUntilQuiet(t, c) {
		if f[0] == serverpackets.OpcodeInventoryUpdate {
			t.Fatal("refused equips queued an InventoryUpdate")
		}
	}

	srv.FlushItems(t)
	if dress := mustFindItem(t, srv, objID, objects[formalWearID]); dress.Location != item.LocationPaperdoll {
		t.Fatalf("dress location after refusals = %v, want paperdoll", dress.Location)
	}
	for _, id := range []int32{leatherHelmetID, shortGlovesID, leatherPantsID} {
		if inst := mustFindItem(t, srv, objID, objects[id]); inst.Location != item.LocationInventory {
			t.Fatalf("refused item %d location = %v, want inventory", id, inst.Location)
		}
	}
}

// TestFormalWearYieldsToHandItems pins the other branch of the guard: a
// weapon or shield takes the dress off first, which relights the skill
// window, and both the dress and the new item move in one InventoryUpdate.
func TestFormalWearYieldsToHandItems(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		id   int32
	}{
		{name: "weapon", id: shortSwordID},
		{name: "shield", id: leatherShieldID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, objects := bootFormalWear(t)
			c := srv.Client
			objID := srv.SoleObjectID(t)
			startInWorld(t, c)

			c.Send(encodeUseItem(objects[formalWearID], false))
			drainUntilQuiet(t, c)
			srv.InventoryUpdates.Tick()
			drainUntilQuiet(t, c)

			c.Send(encodeUseItem(objects[tc.id], false))
			assertSkillListDisabled(t, collectUntilQuiet(t, c), 0)
			srv.InventoryUpdates.Tick()
			frames := collectUntilQuiet(t, c)
			if e := findInventoryUpdate(t, frames, objects[formalWearID]); e.equipped != 0 {
				t.Fatalf("dress equipped flag = %d, want 0", e.equipped)
			}
			if e := findInventoryUpdate(t, frames, objects[tc.id]); e.equipped != 1 {
				t.Fatalf("%s equipped flag = %d, want 1", tc.name, e.equipped)
			}

			srv.FlushItems(t)
			if dress := mustFindItem(t, srv, objID, objects[formalWearID]); dress.Location != item.LocationInventory {
				t.Fatalf("dress location = %v, want inventory", dress.Location)
			}
			if inst := mustFindItem(t, srv, objID, objects[tc.id]); inst.Location != item.LocationPaperdoll {
				t.Fatalf("%s location = %v, want paperdoll", tc.name, inst.Location)
			}
		})
	}
}

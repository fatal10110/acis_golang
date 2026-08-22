package skills

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// wireReader starts a payload reader over a frame's body (opcode excluded).
func wireReader(payload []byte) *wire.Reader {
	return wire.NewReader(payload)
}

// skillPersistence builds the skill layer over the shared MariaDB: saved
// effect rows and known skill levels flow through the real stores, exactly
// like production.
func skillPersistence(t testing.TB, defs []modelskill.Definition) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(defs), gamesql.NewCharacterSkillStore(db))
}

// bookPolicy maps every listed skill to Adena (item id 57): learning one
// requires and consumes one Adena. The wire contract for the requirement
// entry (type 99, item id, count 1, unk 50) is what the oracle sends; the
// item id itself is data-driven.
func bookPolicy(t *testing.T, skillIDs ...modelskill.ID) modelskill.BookPolicy {
	t.Helper()
	books := make([]modelskill.Spellbook, 0, len(skillIDs))
	for _, id := range skillIDs {
		books = append(books, modelskill.Spellbook{SkillID: id, ItemID: 57})
	}
	tbl, err := modelskill.NewSpellbookTable(books)
	if err != nil {
		t.Fatalf("build spellbook table: %v", err)
	}
	return modelskill.BookPolicy{Table: tbl, SPBookNeeded: true, DivineBookNeeded: true}
}

// fishingTrees returns a skill tree with one learnable fishing skill: id
// 1368 (an Expand Inventory level), level 1, min player level 5, costing 2
// of item 57 (Adena in the test item table). 1368 falls in the
// storage-sync range so learning it prompts an ExStorageMaxCount packet.
func fishingTrees() *modelskill.Trees {
	return &modelskill.Trees{Fishing: []modelskill.FishingSkill{
		{ID: 1368, Level: 1, MinLevel: 5, ItemID: 57, ItemCount: 2},
	}}
}

// enchantTree returns the enchant route for skill 1: level requestedLevel
// reachable from any lower level, with the given success rate at character
// level 76 and no item cost (node.ItemID 0 skips the book check).
func enchantTree(requestedLevel, rate int) *modelskill.Trees {
	return &modelskill.Trees{Enchant: []modelskill.EnchantSkill{
		{ID: 1, Level: requestedLevel, Rate76: rate},
	}}
}

// skillListEntry is one decoded SkillList row.
type skillListEntry struct {
	passive int32
	level   int32
	id      int32
}

// assertSkillList asserts frame is a SkillList carrying exactly the given
// entries in order.
func assertSkillList(t *testing.T, frame []byte, want ...skillListEntry) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSkillList, "SkillList")
	r := wireReader(frame[1:])
	if count := r.ReadInt32(); count != int32(len(want)) {
		t.Fatalf("SkillList count = %d, want %d", count, len(want))
	}
	for _, w := range want {
		passive, level, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		if passive != w.passive || level != w.level || id != w.id {
			t.Fatalf("SkillList entry = passive %d level %d id %d, want %d/%d/%d",
				passive, level, id, w.passive, w.level, w.id)
		}
	}
	r.ReadUint8() // canEnchant marker closing the packet
	if err := r.Err(); err != nil {
		t.Fatalf("read SkillList: %v", err)
	}
}

// assertAcquireSkillListEmpty asserts frame is an AcquireSkillList of the
// given trainer type with no remaining offers.
func assertAcquireSkillListEmpty(t *testing.T, frame []byte, wantType serverpackets.AcquireSkillType) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeAcquireSkillList, "AcquireSkillList")
	r := wireReader(frame[1:])
	if skillType, count := r.ReadInt32(), r.ReadInt32(); skillType != int32(wantType) || count != 0 {
		t.Fatalf("AcquireSkillList = type %d count %d, want type %d empty", skillType, count, wantType)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read AcquireSkillList: %v", err)
	}
}

// assertKnownSkills reads the character's persisted known-skill rows and
// compares them with want.
func assertKnownSkills(t *testing.T, srv *gameservertest.Server, objID int32, want map[int]int) {
	t.Helper()
	known, err := srv.KnownSkills.ListKnownSkills(context.Background(), objID, 0)
	if err != nil {
		t.Fatalf("list known skills: %v", err)
	}
	if len(known) != len(want) {
		t.Fatalf("character_skills rows = %+v, want %+v", known, want)
	}
	for id, level := range want {
		if known[id] != level {
			t.Fatalf("character_skills[%d] = %d, want %d (rows %+v)", id, known[id], level, known)
		}
	}
}

// assertSkillListContains asserts frame is a SkillList whose rows include
// the given entry.
func assertSkillListContains(t *testing.T, frame []byte, want skillListEntry) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSkillList, "SkillList")
	r := wireReader(frame[1:])
	count := r.ReadInt32()
	for range count {
		passive, level, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		if passive == want.passive && level == want.level && id == want.id {
			r.ReadUint8() // canEnchant marker closing the packet
			return
		}
	}
	t.Fatalf("SkillList entries (%d rows) missing %+v", count, want)
}

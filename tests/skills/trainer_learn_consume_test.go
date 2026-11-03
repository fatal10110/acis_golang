package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: RequestAcquireSkill.runImpl consumes the spellbook (usual list,
// RequestAcquireSkill.java:84) and the fishing item (:118) through
// Player.destroyItemByItemId(id, count, true) (Player.java:1967-1993), ahead
// of removeExpAndSp, addSkill and LEARNED_SKILL_S1. Its message names what
// went: adena through reduceAdena as S1_DISAPPEARED_ADENA (number), more than
// one unit as S2_S1_DISAPPEARED (item name, item number), one unit as
// S1_DISAPPEARED (item name). A missing item reads NOT_ENOUGH_ITEMS (adena:
// YOU_NOT_ENOUGH_ADENA) ahead of ITEM_MISSING_TO_LEARN_SKILL. The inventory
// update itself waits for the inventory-update task, after the reply.

// spellbookFor returns a book policy where learning skillID needs itemID.
func spellbookFor(t *testing.T, skillID modelskill.ID, itemID int32) modelskill.BookPolicy {
	t.Helper()
	tbl, err := modelskill.NewSpellbookTable([]modelskill.Spellbook{{SkillID: skillID, ItemID: itemID}})
	if err != nil {
		t.Fatalf("build spellbook table: %v", err)
	}
	return modelskill.BookPolicy{Table: tbl, SPBookNeeded: true, DivineBookNeeded: true}
}

// fishingTreeCosting returns the fishing tree of fishingTrees with skill
// 1368 costing count units of itemID.
func fishingTreeCosting(itemID int32, count int) *modelskill.Trees {
	return &modelskill.Trees{Fishing: []modelskill.FishingSkill{
		{ID: 1368, Level: 1, MinLevel: 5, ItemID: itemID, ItemCount: count},
	}}
}

// TestLearnGeneralSpellbookDisappearedPrecedesSPCharge learns a usual skill
// whose spellbook the learner holds: the book's disappeared message comes
// first, then the SP charge, LEARNED_SKILL_S1, the skill list and the list
// close, and the book is gone.
func TestLearnGeneralSpellbookDisappearedPrecedesSPCharge(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		bookID    int32
		held      int32
		messageID int
		params    []systemMessageParam
	}{
		{
			name: "one unit", bookID: 20, held: 2,
			messageID: serverpackets.SystemMessageS1Disappeared,
			params:    []systemMessageParam{{serverpackets.SystemMessageParamItemName, 20}},
		},
		{
			name: "adena", bookID: item.AdenaID, held: 5,
			messageID: serverpackets.SystemMessageS1DisappearedAdena,
			params:    []systemMessageParam{{serverpackets.SystemMessageParamNumber, 1}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, c, objID := bootLearner(t, append(generalLearnOpts(t, 50),
				gameservertest.WithSpellbooks(spellbookFor(t, 3, tc.bookID)))...)
			srv.GiveItem(t, objID, tc.bookID, tc.held)
			startInWorld(t, c)
			selectTrainer(t, srv, c, objID, 0)
			inv := srv.PlayerInventory(t, objID)

			c.Send(encodeRequestAcquireSkill(3, 1, 0))
			assertSystemMessageParams(t, c.Read(), tc.messageID, tc.params...)
			assertSPStatus(t, c.Read(), objID, 0)
			assertSystemMessageParams(t, c.Read(), serverpackets.SystemMessageSPDecreasedS1,
				systemMessageParam{serverpackets.SystemMessageParamNumber, 50})
			assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageLearnedSkill, 3, 1)
			assertSkillList(t, c.Read(), skillListEntry{passive: 0, level: 1, id: 3})
			assertEmptyListClose(t, c, serverpackets.SystemMessageNoMoreSkillsToLearn)
			drainUntilQuiet(t, c)

			if got, want := inv.ItemCount(tc.bookID, -1, true), int(tc.held)-1; got != want {
				t.Fatalf("book %d count = %d after learning, want %d", tc.bookID, got, want)
			}
			assertKnownSkills(t, srv, objID, map[int]int{3: 1})
		})
	}
}

// TestLearnFishingItemDisappearedPrecedesLearned learns a fishing skill for
// one, several, and adena units: the disappeared message naming them comes
// ahead of LEARNED_SKILL_S1 and the storage resync, and the units are gone.
func TestLearnFishingItemDisappearedPrecedesLearned(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		itemID    int32
		count     int
		messageID int
		params    []systemMessageParam
	}{
		{
			name: "one unit", itemID: 20, count: 1,
			messageID: serverpackets.SystemMessageS1Disappeared,
			params:    []systemMessageParam{{serverpackets.SystemMessageParamItemName, 20}},
		},
		{
			name: "several units", itemID: 20, count: 3,
			messageID: serverpackets.SystemMessageS2S1Disappeared,
			params: []systemMessageParam{
				{serverpackets.SystemMessageParamItemName, 20},
				{serverpackets.SystemMessageParamItemNumber, 3},
			},
		},
		{
			name: "adena", itemID: item.AdenaID, count: 2,
			messageID: serverpackets.SystemMessageS1DisappearedAdena,
			params:    []systemMessageParam{{serverpackets.SystemMessageParamNumber, 2}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, c, objID := bootLearner(t,
				gameservertest.WithCharacter("Newbie", 5, 50),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(learnerTable(t, modelskill.Definition{ID: 1368, Level: 1, Activation: modelskill.ActivationActive})),
				gameservertest.WithSkillTrees(fishingTreeCosting(tc.itemID, tc.count)))
			const held = 5
			srv.GiveItem(t, objID, tc.itemID, held)
			startInWorld(t, c)
			selectTrainer(t, srv, c, objID)
			inv := srv.PlayerInventory(t, objID)

			c.Send(encodeRequestAcquireSkill(1368, 1, 1))
			assertSystemMessageParams(t, c.Read(), tc.messageID, tc.params...)
			assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageLearnedSkill, 1368, 1)
			reply := c.Read()
			assertFrameOpcode(t, reply, serverpackets.OpcodeExtended, "ExStorageMaxCount extended")
			if sub := wireReader(reply[1:]).ReadUint16(); sub != serverpackets.OpcodeExStorageMaxCount {
				t.Fatalf("extended opcode = %#x, want ExStorageMaxCount (%#x)", sub, serverpackets.OpcodeExStorageMaxCount)
			}
			assertSkillList(t, c.Read(), skillListEntry{passive: 0, level: 1, id: 1368})
			assertEmptyListClose(t, c, serverpackets.SystemMessageNoMoreSkillsToLearn)
			drainUntilQuiet(t, c)

			if got, want := inv.ItemCount(tc.itemID, -1, true), held-tc.count; got != want {
				t.Fatalf("item %d count = %d after learning, want %d", tc.itemID, got, want)
			}
			assertKnownSkills(t, srv, objID, map[int]int{1368: 1})
		})
	}
}

// TestLearnMissingItemNamesShortfallFirst learns with too few of the
// required item on both lists: NOT_ENOUGH_ITEMS precedes
// ITEM_MISSING_TO_LEARN_SKILL and the reopened list, nothing is taken and
// nothing is learned.
func TestLearnMissingItemNamesShortfallFirst(t *testing.T) {
	t.Parallel()
	t.Run("spellbook", func(t *testing.T) {
		t.Parallel()
		srv, c, objID := bootLearner(t, append(generalLearnOpts(t, 50),
			gameservertest.WithSpellbooks(spellbookFor(t, 3, 20)))...)
		startInWorld(t, c)
		selectTrainer(t, srv, c, objID, 0)

		c.Send(encodeRequestAcquireSkill(3, 1, 0))
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageNotEnoughItems)
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageItemMissingToLearnSkill)
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeAcquireSkillList, "AcquireSkillList")
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "trainer list ActionFailed")
		drainUntilQuiet(t, c)
		assertLiveSP(t, srv, objID, 50)
		assertKnownSkills(t, srv, objID, map[int]int{})
	})
	t.Run("fishing", func(t *testing.T) {
		t.Parallel()
		srv, c, objID := bootLearner(t,
			gameservertest.WithCharacter("Newbie", 5, 50),
			gameservertest.WithWantChars(1),
			gameservertest.WithSkills(learnerTable(t, modelskill.Definition{ID: 1368, Level: 1, Activation: modelskill.ActivationActive})),
			gameservertest.WithSkillTrees(fishingTreeCosting(20, 3)))
		srv.GiveItem(t, objID, 20, 2)
		startInWorld(t, c)
		selectTrainer(t, srv, c, objID)
		inv := srv.PlayerInventory(t, objID)

		c.Send(encodeRequestAcquireSkill(1368, 1, 1))
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageNotEnoughItems)
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageItemMissingToLearnSkill)
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeAcquireSkillList, "AcquireSkillList")
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "fishing list ActionFailed")
		drainUntilQuiet(t, c)
		if got := inv.ItemCount(20, -1, true); got != 2 {
			t.Fatalf("item 20 count = %d after the refused learn, want 2", got)
		}
		assertKnownSkills(t, srv, objID, map[int]int{})
	})
}

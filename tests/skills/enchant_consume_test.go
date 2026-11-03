package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: RequestExEnchantSkill.runImpl consumes the enchant book through
// Player.destroyItemByItemId(id, count, true) (RequestExEnchantSkill.java:71,
// Player.java:1967-1993) after the SP and exp checks and ahead of
// removeExpAndSp. Its message names what went: adena through reduceAdena as
// S1_DISAPPEARED_ADENA (number), more than one unit as S2_S1_DISAPPEARED
// (item name, item number), one unit as S1_DISAPPEARED (item name). A
// missing book reads NOT_ENOUGH_ITEMS (adena: YOU_NOT_ENOUGH_ADENA) ahead of
// YOU_DONT_HAVE_ALL_OF_THE_ITEMS_NEEDED_TO_ENCHANT_THAT_SKILL.

// enchantSPCost is the SP the consuming enchant tree charges.
const enchantSPCost = 50

// enchantTreeCosting returns a certain-success enchant to skill 1 level 101
// costing enchantSPCost SP and count units of itemID.
func enchantTreeCosting(itemID int32, count int) *modelskill.Trees {
	return &modelskill.Trees{Enchant: []modelskill.EnchantSkill{
		{ID: 1, Level: 101, SP: enchantSPCost, Rate76: 100, ItemID: itemID, ItemCount: count},
	}}
}

// TestEnchantSkillBookDisappearedPrecedesSPCharge enchants a skill whose
// book the player holds, for one, several and adena units: the message
// naming what went comes first, then the SP charge and the enchant result,
// and the units are gone.
func TestEnchantSkillBookDisappearedPrecedesSPCharge(t *testing.T) {
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
			srv, c, objID := seedEnchanter(t, 1, gameservertest.WithSkillTrees(enchantTreeCosting(tc.itemID, tc.count)))
			const held = 5
			srv.GiveItem(t, objID, tc.itemID, held)
			startInWorld(t, c)
			selectTrainer(t, srv, c, objID, duelistParentClass)
			inv := srv.PlayerInventory(t, objID)

			c.Send(encodeRequestExEnchantSkill(1, 101))
			assertSystemMessageParams(t, c.Read(), tc.messageID, tc.params...)
			assertSPStatus(t, c.Read(), objID, 1_000_000-enchantSPCost)
			assertSystemMessageParams(t, c.Read(), serverpackets.SystemMessageSPDecreasedS1,
				systemMessageParam{serverpackets.SystemMessageParamNumber, enchantSPCost})
			assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageSucceededEnchantingSkillS1, 1, 101)
			assertSkillListContains(t, c.Read(), skillListEntry{passive: 1, level: 101, id: 1})
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeUserInfo, "post-enchant UserInfo")
			drainUntilQuiet(t, c)

			if got, want := inv.ItemCount(tc.itemID, -1, true), held-tc.count; got != want {
				t.Fatalf("item %d count = %d after enchanting, want %d", tc.itemID, got, want)
			}
			assertKnownSkills(t, srv, objID, map[int]int{1: 101})
		})
	}
}

// TestEnchantSkillMissingBookNamesShortfallFirst enchants holding too few
// units of the book (or adena): the shortfall message precedes the enchant
// rejection, and nothing is taken, charged or enchanted.
func TestEnchantSkillMissingBookNamesShortfallFirst(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		itemID    int32
		messageID int
	}{
		{name: "book", itemID: 20, messageID: serverpackets.SystemMessageNotEnoughItems},
		{name: "adena", itemID: item.AdenaID, messageID: serverpackets.SystemMessageYouNotEnoughAdena},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, c, objID := seedEnchanter(t, 1, gameservertest.WithSkillTrees(enchantTreeCosting(tc.itemID, 3)))
			const held = 2
			srv.GiveItem(t, objID, tc.itemID, held)
			startInWorld(t, c)
			selectTrainer(t, srv, c, objID, duelistParentClass)
			inv := srv.PlayerInventory(t, objID)

			c.Send(encodeRequestExEnchantSkill(1, 101))
			assertStaticSystemMessage(t, c.Read(), tc.messageID)
			assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageMissingItemsToEnchantSkill)
			drainUntilQuiet(t, c)

			if got := inv.ItemCount(tc.itemID, -1, true); got != held {
				t.Fatalf("item %d count = %d after the refused enchant, want %d", tc.itemID, got, held)
			}
			assertLiveSP(t, srv, objID, 1_000_000)
			assertKnownSkills(t, srv, objID, map[int]int{1: 1})
		})
	}
}

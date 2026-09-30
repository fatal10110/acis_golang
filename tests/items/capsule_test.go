package items

import (
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: a capsule is an ItemSkills item whose skill has the
// EXTRACTABLE type (for example item 5916 and skill 2171, Spellbook Paper).
// The cast spends the capsule (PlayableCast.doCast), then Extractable.java:
// 26-66 rolls Rnd.get(100000) over the product rows, subtracting each row's
// chance * 1000. For the row that takes the roll below zero,
// validateCapacityByItemIds sums calculateUsedSlots over its items (0 for a
// held stackable, 1 for a new stack, count for a non-stackable;
// PcInventory.java:537-544, 578-586) and on failure sends SLOTS_FULL (129)
// and creates nothing. Otherwise every item goes through
// Player.addItem(itemId, count, true): YOU_PICKED_UP_S2_S1 (29) with the
// item name and the count as an item number for more than one unit,
// YOU_PICKED_UP_S1 (30) otherwise (Player.java:1806-1852), and
// ItemContainer.addItem creates one instance per unit of a non-stackable
// under the default MultipleItemDrop = True (ItemContainer.java:224-253).
// A roll that takes no row sends NOTHING_INSIDE_THAT (1669).

const capsuleSkillID = 2171

const (
	capsuleItemID int32 = 9950
	// capsuleWeaponID is the catalog's non-stackable weapon.
	capsuleWeaponID int32 = 30
	// capsulePotionID is the catalog's stackable potion.
	capsulePotionID int32 = 20
)

// capsuleSkills is a skill table holding the capsule's EXTRACTABLE skill
// with products.
func capsuleSkills(t *testing.T, products string) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: 248, Level: 3},
		{ID: 294, Level: 1},
		{
			ID: capsuleSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "EXTRACTABLE", MagicLevel: 1, StaticHitTime: true, HitTime: 500, StaticReuse: true,
			ExtractableItems: products,
		},
	}), gamesql.NewCharacterSkillStore(db))
}

// capsuleCatalog is the shared item catalog plus a stackable capsule that
// casts the capsule skill.
func capsuleCatalog() *item.Table {
	templates := gameservertest.ItemTemplates().All()
	templates = append(templates, &item.Template{
		ID: capsuleItemID, Name: "Test Capsule", Kind: item.KindEtcItem, Duration: -1,
		Stackable: true, Destroyable: true,
		EtcItem:        &item.EtcItemDetail{Type: item.EtcItemScroll, Handler: "ItemSkills", SharedReuseGroup: -1},
		AttachedSkills: []item.SkillRef{{ID: capsuleSkillID, Level: 1}},
	})
	return item.NewTable(templates)
}

// openCapsule boots a player holding two capsules and one weapon with the
// given slot limit, opens one capsule, and returns every frame from the
// cast's launch until the client goes quiet, the InventoryUpdate tick
// still pending.
func openCapsule(t *testing.T, products string, slots int) (*gameservertest.Server, int32, [][]byte) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(capsuleCatalog()),
		gameservertest.WithSkills(capsuleSkills(t, products)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithInventorySlots(slots, slots),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	capsule := srv.GiveItem(t, objID, capsuleItemID, 2)
	srv.GiveItem(t, objID, capsuleWeaponID, 1)
	startInWorld(t, c)

	c.Send(encodeUseItem(capsule, false))
	assertMagicSkillUseSelf(t, c.Read(), objID, capsuleSkillID, 1, 500, 0)
	readUntilLaunched(t, c)
	// The capsule opens at the hit, after the launch.
	first := c.ReadWithTimeout(2 * time.Second)
	if first == nil {
		t.Fatal("nothing followed the capsule cast's launch")
	}
	return srv, objID, append([][]byte{first}, collectUntilQuiet(t, c)...)
}

// assertOneCapsuleSpent requires one of the two capsules gone: opening
// spends it whatever it yields.
func assertOneCapsuleSpent(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	if got := carriedCount(t, srv, objID, capsuleItemID); got != 1 {
		t.Fatalf("capsules left = %d, want 1", got)
	}
}

// readUntilLaunched reads the cast's frames up to its MagicSkillLaunched.
func readUntilLaunched(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	for range 20 {
		frame := c.ReadWithTimeout(2 * time.Second)
		if frame == nil {
			break
		}
		if frame[0] == serverpackets.OpcodeMagicSkillLaunched {
			return
		}
	}
	t.Fatal("capsule cast never launched")
}

// TestCapsuleGrantsTheRolledRow opens a capsule whose only row always wins:
// five potions, a new stack, and two weapons, one instance each. Each item
// is named as picked up, the stack's count as an item number, and every
// new instance reaches the database.
func TestCapsuleGrantsTheRolledRow(t *testing.T) {
	t.Parallel()
	// Capsules and the held weapon take two slots; the potion stack and
	// two more weapons need three more.
	srv, objID, frames := openCapsule(t, "20,5,30,2,100", 5)
	assertOneCapsuleSpent(t, srv, objID)

	messages := systemMessages(frames)
	if len(messages) != 2 {
		t.Fatalf("system messages = %d, want the two product lines", len(messages))
	}
	assertGrantMessage(t, messages[0], serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(capsulePotionID), itemNumberParam(5))
	assertGrantMessage(t, messages[1], serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(capsuleWeaponID), itemNumberParam(2))

	if got := carriedCount(t, srv, objID, capsulePotionID); got != 5 {
		t.Fatalf("carried potions = %d, want 5", got)
	}
	if got := carriedCount(t, srv, objID, capsuleWeaponID); got != 3 {
		t.Fatalf("carried weapons = %d, want 3", got)
	}
	if got := srv.PlayerInventory(t, objID).Size(); got != 5 {
		t.Fatalf("inventory size = %d, want 5: one potion stack and one instance per weapon", got)
	}

	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, srv.Client)
	srv.FlushItems(t)
	potions, weapons := 0, 0
	for _, inst := range persistedItems(t, srv, objID) {
		switch inst.TemplateID {
		case capsulePotionID:
			potions += inst.Count
		case capsuleWeaponID:
			weapons++
		}
	}
	if potions != 5 || weapons != 3 {
		t.Fatalf("persisted potions = %d, weapon rows = %d; want 5 and 3", potions, weapons)
	}
}

// TestCapsuleSingleUnitNamesTheItem: one unit of a product reads
// YOU_PICKED_UP_S1 with the item name alone.
func TestCapsuleSingleUnitNamesTheItem(t *testing.T) {
	t.Parallel()
	srv, objID, frames := openCapsule(t, "20,1,100", 5)
	assertOneCapsuleSpent(t, srv, objID)

	messages := systemMessages(frames)
	if len(messages) != 1 {
		t.Fatalf("system messages = %d, want the product line", len(messages))
	}
	assertGrantMessage(t, messages[0], serverpackets.SystemMessageYouPickedUpS1, itemNameParam(capsulePotionID))
	if got := carriedCount(t, srv, objID, capsulePotionID); got != 1 {
		t.Fatalf("carried potions = %d, want 1", got)
	}
}

// TestCapsuleRollWithoutRowFindsNothingInside: a row with no chance never
// takes the roll, so the capsule is spent for NOTHING_INSIDE_THAT alone.
func TestCapsuleRollWithoutRowFindsNothingInside(t *testing.T) {
	t.Parallel()
	srv, objID, frames := openCapsule(t, "20,5,0", 5)
	assertOneCapsuleSpent(t, srv, objID)

	messages := systemMessages(frames)
	if len(messages) != 1 {
		t.Fatalf("system messages = %d, want NOTHING_INSIDE_THAT alone", len(messages))
	}
	assertStaticSystemMessage(t, messages[0], serverpackets.SystemMessageNothingInsideThat)
	if got := carriedCount(t, srv, objID, capsulePotionID); got != 0 {
		t.Fatalf("carried potions = %d, want none", got)
	}
}

// TestCapsuleAtSlotLimitCreatesNothing opens a capsule whose rolled row
// needs more slots than are free. The count is per item and quantity: two
// new ids take two slots, and two units of a non-stackable take two slots
// although they share one id. Either way the capsule is spent for
// SLOTS_FULL and nothing is created.
func TestCapsuleAtSlotLimitCreatesNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, products string
	}{
		{"two new stacks", "20,5,9001,1,100"},
		{"two units of a non-stackable", "30,2,100"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Three slots: capsules and the held weapon leave one free.
			srv, objID, frames := openCapsule(t, tc.products, 3)
			assertOneCapsuleSpent(t, srv, objID)

			messages := systemMessages(frames)
			if len(messages) != 1 {
				t.Fatalf("system messages = %d, want SLOTS_FULL alone", len(messages))
			}
			assertStaticSystemMessage(t, messages[0], serverpackets.SystemMessageSlotsFull)
			if got := srv.PlayerInventory(t, objID).Size(); got != 2 {
				t.Fatalf("inventory size = %d, want 2: nothing created", got)
			}
			if got := carriedCount(t, srv, objID, capsuleWeaponID); got != 1 {
				t.Fatalf("carried weapons = %d, want the held one", got)
			}
		})
	}
}

// TestCapsuleHeldStackNeedsNoSlot: a product merging into a held stack
// needs no slot, so it arrives at the slot limit.
func TestCapsuleHeldStackNeedsNoSlot(t *testing.T) {
	t.Parallel()
	// Two slots: capsules and the held weapon fill them; the product is
	// more capsules, which join the held stack.
	srv, objID, frames := openCapsule(t, "9950,3,100", 2)

	messages := systemMessages(frames)
	if len(messages) != 1 {
		t.Fatalf("system messages = %d, want the product line", len(messages))
	}
	assertGrantMessage(t, messages[0], serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(capsuleItemID), itemNumberParam(3))
	if got := carriedCount(t, srv, objID, capsuleItemID); got != 4 {
		t.Fatalf("capsules = %d, want 1 left plus 3 created", got)
	}
	if got := srv.PlayerInventory(t, objID).Size(); got != 2 {
		t.Fatalf("inventory size = %d, want 2", got)
	}
}

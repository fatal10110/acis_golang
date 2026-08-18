package network

import (
	"bytes"
	"testing"

	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

// consumableSkillTable seeds the two instant-cast potion skills the item
// templates in testItemTemplates reference: a heal-over-time potion and a
// percent-of-max mana potion. Both are flagged as potions so the use-item
// path applies them instantly to the user.
func consumableSkillTable(t *testing.T) *skillstate.Persistence {
	t.Helper()
	store := newMemorySkillSaveStore()
	return skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 2031, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "HOT", Potion: true, HitTime: 0,
			Effects: []modelskill.EffectTemplate{{Name: "HealOverTime", Count: 7, Time: 2, Value: 16, Icon: true}},
		},
		{
			ID: 2279, Level: 2, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "MANAHEAL_PERCENT", Potion: true, Power: 20, HitTime: 0,
		},
		{
			ID: 2165, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "UNLOCK", Potion: true, HitTime: 0, NumCharges: 1, MaxCharges: 2,
		},
	}), store)
}

func TestSendItemSkillConditionFailureAddsSkillName(t *testing.T) {
	capture := &frameCapture{}
	live := newTestLivePlayer(t, 7, capture)

	sendItemSkillConditionFailure(live, itemhandler.UseResult{
		Skill: modelskill.Definition{ID: 2278, Level: 1},
		Condition: modelskill.ConditionClause{
			MessageID: serverpackets.SystemMessageS1CannotBeUsed,
			AddName:   true,
		},
	})

	if len(capture.frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(capture.frames))
	}
	assertSystemMessageSkillFrame(t, capture.frames[0], serverpackets.SystemMessageS1CannotBeUsed, 2278, 1)
}

// TestGameClientLinkUsePotionNotEnoughItemsSendsNotEnoughItems verifies a
// potion whose stack destroy fails (e.g. a use/pickup race) is rejected with
// NOT_ENOUGH_ITEMS (351), matching Java's PlayableCast.doInstantCast
// destroyItem failure — not S1_CANNOT_BE_USED, which is reserved for the
// skill's own itemConsumeId precheck on a player-requested cast.
func TestGameClientLinkUsePotionNotEnoughItemsSendsNotEnoughItems(t *testing.T) {
	skills := consumableSkillTable(t)
	capture := &frameCapture{}
	live := newTestLivePlayer(t, 7, capture)
	link := &GameClientLink{skills: skills}

	const potionTemplate int32 = 1060
	const objectID int32 = 704
	// inst is never added to live's inventory container, so the
	// destroyer's ItemByObjectID lookup misses and DestroyItem fails,
	// reproducing the race without a full item-store round trip.
	inst := &item.Instance{
		ObjectID: objectID, TemplateID: potionTemplate, OwnerID: live.ObjectID(),
		Count: 1, Location: item.LocationInventory, ManaLeft: -1,
	}

	if link.useConsumableSkillItem(live, live.Inventory(), inst) != true {
		t.Fatal("useConsumableSkillItem() = false, want true (handled)")
	}

	if got, want := frameOpcodes(capture.frames), []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}; !bytes.Equal(got, want) {
		t.Fatalf("destroy-failure opcodes = %#x, want SystemMessage then ActionFailed (%#x)", got, want)
	}
	assertSystemMessageIDFrame(t, capture.frames[0], serverpackets.SystemMessageNotEnoughItems)
}

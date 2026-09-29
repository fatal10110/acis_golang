package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: PlayableCast.canCast (PlayableCast.java:88-96) checks a skill's
// consume item against the acting player's inventory, the owner's for a
// summon, and refuses with S1_CANNOT_BE_USED naming the skill. Only
// PlayerCast.doCast (PlayerCast.java:181-182) destroys the item, so a
// summon's cast consumes nothing.

// strikeItemID is the shared catalog's stackable Potion, the strike's
// consume item in these scenarios.
const strikeItemID = int32(20)

// TestSummonItemCostChecksOwnerStack commands a strike that needs three of
// the owner's potions. With five the pet casts and the owner keeps all five,
// reading no item message; with two the owner reads S1_CANNOT_BE_USED naming
// the strike and no cast starts.
func TestSummonItemCostChecksOwnerStack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		held  int32
		casts bool
	}{
		{name: "owner holds enough", held: 5, casts: true},
		{name: "owner holds too few", held: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			strike := wolfStrike()
			strike.ItemConsumeID, strike.ItemConsumeCount = int(strikeItemID), 3
			h, petActor, hostile := bootWolfStrikerSeeded(t, strike, []seedItem{{TemplateID: strikeItemID, Count: tc.held}})

			runOnPetQueue(t, petActor, func() { petActor.TryUseSkill(wolfStrikeSkill, hostile, false) })
			if !tc.casts {
				assertSummonCastRejected(t, h, petActor, hostile.ObjectID(), func(f []byte) {
					assertSystemMessageSkill(t, f, serverpackets.SystemMessageS1CannotBeUsed, wolfStrikeSkill, 1)
				})
				if petActor.CastingNow() {
					t.Fatal("refused strike left the pet casting")
				}
			} else {
				frames := readUntilOpcode(t, h.client, serverpackets.OpcodeMagicSkillUse, "strike MagicSkillUse")
				requireSkillUseOnto(t, frames, petActor, hostile.ObjectID())
				if !petActor.CastingNow() {
					t.Fatal("strike did not leave the pet casting")
				}
				for _, f := range append(frames, drainFrames(t, h.client)...) {
					if f[0] != serverpackets.OpcodeSystemMessage {
						continue
					}
					switch id := int(wire.NewReader(f[1:]).ReadInt32()); id {
					case serverpackets.SystemMessageS1Disappeared, serverpackets.SystemMessageS2S1Disappeared, serverpackets.SystemMessageNotEnoughItems:
						t.Fatalf("summon strike sent item message %d to the owner, want none", id)
					}
				}
			}
			if got := h.srv.PlayerInventory(t, h.ownerID).ItemCount(strikeItemID, -1, true); got != int(tc.held) {
				t.Fatalf("owner potions = %d after the strike, want %d kept", got, tc.held)
			}
		})
	}
}

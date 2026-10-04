package ai

import "testing"

// TestIntentionEnumName pins each intention to its IntentionType constant
// name, and an unknown value to none.
func TestIntentionEnumName(t *testing.T) {
	for i, want := range map[Intention]string{
		IntentionIdle: "IDLE", IntentionAttack: "ATTACK", IntentionCast: "CAST", IntentionFakeDeath: "FAKE_DEATH",
		IntentionFlee: "FLEE", IntentionFollow: "FOLLOW", IntentionInteract: "INTERACT", IntentionMoveRoute: "MOVE_ROUTE",
		IntentionMoveTo: "MOVE_TO", IntentionNothing: "NOTHING", IntentionPickUp: "PICK_UP", IntentionSit: "SIT",
		IntentionSocial: "SOCIAL", IntentionStand: "STAND", IntentionUseItem: "USE_ITEM", IntentionWander: "WANDER",
		IntentionWander + 1: "",
	} {
		if got := i.EnumName(); got != want {
			t.Errorf("Intention(%d).EnumName() = %q, want %q", i, got, want)
		}
	}
}

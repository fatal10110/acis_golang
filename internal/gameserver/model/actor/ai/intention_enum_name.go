package ai

// intentionEnumNames is ordered by Intention value: each intention's
// constant name in the reference IntentionType enum, the spelling admin
// pages print.
var intentionEnumNames = [...]string{
	"IDLE", "ATTACK", "CAST", "FAKE_DEATH", "FLEE", "FOLLOW", "INTERACT", "MOVE_ROUTE",
	"MOVE_TO", "NOTHING", "PICK_UP", "SIT", "SOCIAL", "STAND", "USE_ITEM", "WANDER",
}

// EnumName returns i's constant name in the reference IntentionType enum,
// empty for a value past the last intention.
func (i Intention) EnumName() string {
	if int(i) < len(intentionEnumNames) {
		return intentionEnumNames[i]
	}
	return ""
}

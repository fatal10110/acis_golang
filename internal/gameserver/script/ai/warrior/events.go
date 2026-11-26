package warrior

// The script event ids a warrior sends and answers, by what the event
// carries.
const (
	eventFleeHelp     = 10000 // a fleeing monster names, in Arg1, the creature its clan turns on
	eventHelp         = 10001 // an ally under attack calls for help against the creature in Arg1
	eventMasterTarget = 10002 // a party leader names, in Arg1, the target its party turns on
	eventShoutTarget  = 10016 // a shouting monster names its target
	eventAttackPet    = 10017 // names, in Arg1, a creature to attack
	eventGrown        = 10018 // a grown beast tells the beasts around it that it appeared
	eventCallChildren = 10019 // a parent calls its children to the creature in Arg1
	eventTarget       = 10020 // a caller hands its target over, or asks to look around
	eventDismiss      = 10023 // a caller dismisses the guards it summoned
	eventLeaderLeft   = 10028 // a leader tells its party that it left
	eventHelpCall     = 10033 // a heretic calls for help against the creature in Arg1
	eventHealMaster   = 10034 // a leader under 80% of its HP asks its party for a heal
	eventWalkHome     = 10035 // a heretic leader calls its party home
)

package wizard

// The script event ids a wizard sends and answers, by what the event
// carries.
const (
	eventFleeHelp     = 10000 // a fleeing monster names, in Arg1, the creature its clan turns on
	eventMasterTarget = 10002 // a party leader names, in Arg1, the target its party turns on
	eventShoutTarget  = 10016 // a shouting monster names its target
	eventGrown        = 10018 // a grown beast tells the beasts around it that it appeared
	eventTarget       = 10020 // a caller hands its target over, or asks to look around
	eventHelpCall     = 10033 // a heretic calls for help against the creature in Arg1
	eventWalkHome     = 10035 // a heretic leader calls its party home
)

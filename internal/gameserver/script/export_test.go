package script

// NPCEventCount is the number of NPC events, for tests outside the package.
const NPCEventCount = npcEventCount

// RaiseAll returns cfg with every hook raised on every NPC kind, for tests
// that replay registrations without the seam gate.
func RaiseAll(cfg Config) Config {
	cfg.raises = func(hook, NPCKind) bool { return true }
	return cfg
}

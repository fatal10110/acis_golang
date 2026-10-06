package script

// Script is one scripts.xml entry built in Go: what it binds to and the
// hooks it reacts with. A behavior derives its hooks from its parent's with
// Hooks.With; calling the parent is calling the captured parent Hooks.
type Script struct {
	// Name is the last element of the script's scripts.xml path; Build sets
	// it. It keys the script's character_quests rows.
	Name string
	// QuestID, when positive, makes the script a real quest.
	QuestID int32
	// Items are the item ids the quest takes from the player when it ends.
	Items []int32
	// Bind lists, per NPC event, the NPC ids the script registers for
	// explicitly: quests, features, teleporters, and behaviors that bind
	// more than their own events.
	Bind Bindings
	// Behavior marks an NPC behavior: an NPC event holds at most one
	// behavior, the last one listed.
	Behavior bool
	// NPCs are a behavior's own NPC ids. Each one is bound to every NPC
	// event whose hook is set after With; a parent's ids are never
	// inherited. Only a behavior's NPCs bind anything.
	NPCs []int32
	Hooks

	// path is the scripts.xml path the script was registered under.
	path string
	// env is what the script's helpers act through; Build sets it.
	env *Env
	// timers are the timers of every script; Build sets them.
	timers *timers
}

// Bindings maps an NPC event to the NPC ids a script registers for.
type Bindings map[NPCEvent][]int32

// Catalog maps a scripts.xml path to the constructor of its script. Only
// listed paths are ever constructed.
type Catalog map[string]func() Script

// Listing is one scripts.xml entry, in file order. Schedule, Start and End
// are the entry's schedule attributes as written, empty when absent; only
// scheduled tasks read them.
type Listing struct {
	Path     string
	Schedule string
	Start    string
	End      string
}

// NPCEvent is an NPC event a script registers for. The order is the
// registration manifest's.
type NPCEvent uint8

// The NPC events.
const (
	EventAbnormalStatusChanged NPCEvent = iota
	EventAttacked
	EventAttackFinished
	EventClanAttacked
	EventClanDied
	EventCreated
	EventDecayed
	EventFirstTalk
	EventMoveToFinished
	EventMyDying
	EventNoDesire
	EventOutOfTerritory
	EventPartyAttacked
	EventPartyDied
	EventPickedItem
	EventQuestStart
	EventSeeCreature
	EventSeeItem
	EventSeeSpell
	EventSpelled
	EventStaticObjectClanAttacked
	EventTalked
	EventUseSkillFinished
	EventScriptEvent
	npcEventCount
)

// npcEvents describes each NPC event: its manifest name, the hook that
// answers it, and whether a behavior is bound to it by setting that hook.
// QUEST_START and TALKED are bound only explicitly; both answer with the
// talk hook.
var npcEvents = [npcEventCount]struct {
	name   string
	hook   hook
	byHook bool
}{
	EventAbnormalStatusChanged:    {"ABNORMAL_STATUS_CHANGED", hookAbnormalStatusChanged, true},
	EventAttacked:                 {"ATTACKED", hookAttacked, true},
	EventAttackFinished:           {"ATTACK_FINISHED", hookAttackFinished, true},
	EventClanAttacked:             {"CLAN_ATTACKED", hookClanAttacked, true},
	EventClanDied:                 {"CLAN_DIED", hookClanDied, true},
	EventCreated:                  {"CREATED", hookCreated, true},
	EventDecayed:                  {"DECAYED", hookDecayed, true},
	EventFirstTalk:                {"FIRST_TALK", hookFirstTalk, true},
	EventMoveToFinished:           {"MOVE_TO_FINISHED", hookMoveToFinished, true},
	EventMyDying:                  {"MY_DYING", hookMyDying, true},
	EventNoDesire:                 {"NO_DESIRE", hookNoDesire, true},
	EventOutOfTerritory:           {"OUT_OF_TERRITORY", hookOutOfTerritory, true},
	EventPartyAttacked:            {"PARTY_ATTACKED", hookPartyAttacked, true},
	EventPartyDied:                {"PARTY_DIED", hookPartyDied, true},
	EventPickedItem:               {"PICKED_ITEM", hookPickedItem, true},
	EventQuestStart:               {"QUEST_START", hookTalk, false},
	EventSeeCreature:              {"SEE_CREATURE", hookSeeCreature, true},
	EventSeeItem:                  {"SEE_ITEM", hookSeeItem, true},
	EventSeeSpell:                 {"SEE_SPELL", hookSeeSpell, true},
	EventSpelled:                  {"SPELLED", hookSpelled, true},
	EventStaticObjectClanAttacked: {"STATIC_OBJECT_CLAN_ATTACKED", hookStaticObjectClanAttacked, true},
	EventTalked:                   {"TALKED", hookTalk, false},
	EventUseSkillFinished:         {"USE_SKILL_FINISHED", hookUseSkillFinished, true},
	EventScriptEvent:              {"SCRIPT_EVENT", hookScriptEvent, true},
}

// String returns the event's manifest name.
func (e NPCEvent) String() string {
	if e < npcEventCount {
		return npcEvents[e].name
	}
	return "NPCEvent(?)"
}

// single reports whether the event holds one script per NPC: first talk.
func (e NPCEvent) single() bool { return e == EventFirstTalk }

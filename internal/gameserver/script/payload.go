package script

// NPC is a script's handle on one NPC.
type NPC struct{}

// Player is a script's handle on one player.
type Player struct{}

// Creature is a script's handle on any creature: an NPC or a player.
type Creature interface{ creature() }

func (*NPC) creature()    {}
func (*Player) creature() {}

// The hook payloads, one per fact. Each carries the handles and values the
// hook reacts to. A skill, item or door the fact also involves is added, in
// its script-facing form, by the change that raises the hook.

// AbnormalStatusChanged: an effect is about to land on NPC.
type AbnormalStatusChanged struct {
	NPC    *NPC
	Caster Creature
}

// Attacked: Attacker hit NPC for Damage.
type Attacked struct {
	NPC      *NPC
	Attacker Creature
	Damage   int32
}

// AttackFinished: NPC finished an attack on Target.
type AttackFinished struct {
	NPC    *NPC
	Target Creature
}

// ClanAttacked: Caller, attacked by Attacker, called its clan member Called.
type ClanAttacked struct {
	Caller, Called *NPC
	Attacker       Creature
	Damage         int32
}

// ClanDied: Caller, killed by Killer, told its clan member Called.
type ClanDied struct {
	Caller, Called *NPC
	Killer         Creature
}

// Created: NPC spawned or respawned.
type Created struct{ NPC *NPC }

// Decayed: NPC's corpse decayed.
type Decayed struct{ NPC *NPC }

// Event: Player sent the script event Name, through NPC when NPC is set.
type Event struct {
	Name   string
	NPC    *NPC
	Player *Player
}

// FirstTalk: Player opened NPC's first dialog.
type FirstTalk struct {
	NPC    *NPC
	Player *Player
}

// MoveToFinished: NPC arrived at X, Y, Z.
type MoveToFinished struct {
	NPC     *NPC
	X, Y, Z int32
}

// MyDying: Killer killed NPC.
type MyDying struct {
	NPC    *NPC
	Killer Creature
}

// NoDesire: NPC has nothing to do.
type NoDesire struct{ NPC *NPC }

// OutOfTerritory: NPC left its territory.
type OutOfTerritory struct{ NPC *NPC }

// PartyAttacked: Caller, attacked by Target, called its party member
// Called.
type PartyAttacked struct {
	Caller, Called *NPC
	Target         Creature
	Damage         int32
}

// PartyDied: Caller died and told its party member Called.
type PartyDied struct{ Caller, Called *NPC }

// PickedItem: NPC picked up an item.
type PickedItem struct{ NPC *NPC }

// ScriptEvent: NPC received the script event EventID with two arguments.
type ScriptEvent struct {
	NPC        *NPC
	EventID    int32
	Arg1, Arg2 int32
}

// SeeCreature: NPC noticed Creature.
type SeeCreature struct {
	NPC      *NPC
	Creature Creature
}

// SeeItem: NPC noticed Quantity items.
type SeeItem struct {
	NPC      *NPC
	Quantity int32
}

// SeeSpell: NPC saw Caster cast a spell on Targets; IsPet when the caster's
// summon cast it.
type SeeSpell struct {
	NPC     *NPC
	Caster  *Player
	Targets []Creature
	IsPet   bool
}

// Spelled: Caster's spell landed on NPC.
type Spelled struct {
	NPC    *NPC
	Caster *Player
}

// StaticObjectClanAttacked: a door, attacked by Attacker, called the NPC
// Called.
type StaticObjectClanAttacked struct {
	Called   *NPC
	Attacker Creature
	Damage   int32
}

// Talk: Player talked to NPC about the script.
type Talk struct {
	NPC    *NPC
	Player *Player
}

// Timer: the script's timer Name fired, bound to NPC and Player when set.
type Timer struct {
	Name   string
	NPC    *NPC
	Player *Player
}

// UseSkillFinished: NPC finished casting a skill on Creature.
type UseSkillFinished struct {
	NPC      *NPC
	Creature Creature
	Success  bool
}

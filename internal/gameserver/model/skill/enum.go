package skill

import "fmt"

// Activation classifies how a skill turns on: cast on demand, always in
// effect, or switched on and off like a stance.
type Activation uint8

const (
	ActivationPassive Activation = iota
	ActivationActive
	ActivationToggle
)

var activationNames = map[string]Activation{
	"PASSIVE": ActivationPassive,
	"ACTIVE":  ActivationActive,
	"TOGGLE":  ActivationToggle,
}

var activationStrings = [...]string{"PASSIVE", "ACTIVE", "TOGGLE"}

// String returns a's canonical XML spelling.
func (a Activation) String() string {
	if int(a) < len(activationStrings) {
		return activationStrings[a]
	}
	return fmt.Sprintf("Activation(%d)", uint8(a))
}

// Target classifies who or what a skill can be aimed at.
type Target uint8

const (
	TargetNone Target = iota
	TargetSelf
	TargetOne
	TargetParty
	TargetAlly
	TargetClan
	TargetArea
	TargetFrontArea
	TargetAura
	TargetFrontAura
	TargetBehindAura
	TargetCorpse
	TargetUndead
	TargetAuraUndead
	TargetCorpseAlly
	TargetCorpsePlayer
	TargetCorpsePet
	TargetAreaCorpseMob
	TargetCorpseMob
	TargetUnlockable
	TargetHoly
	TargetPartyMember
	TargetPartyOther
	TargetSummon
	TargetAreaSummon
	TargetEnemySummon
	TargetOwnerPet
	TargetGround
)

var targetStrings = [...]string{
	"NONE", "SELF", "ONE", "PARTY", "ALLY", "CLAN", "AREA", "FRONT_AREA",
	"AURA", "FRONT_AURA", "BEHIND_AURA", "CORPSE", "UNDEAD", "AURA_UNDEAD",
	"CORPSE_ALLY", "CORPSE_PLAYER", "CORPSE_PET", "AREA_CORPSE_MOB",
	"CORPSE_MOB", "UNLOCKABLE", "HOLY", "PARTY_MEMBER", "PARTY_OTHER",
	"SUMMON", "AREA_SUMMON", "ENEMY_SUMMON", "OWNER_PET", "GROUND",
}

var targetNames = func() map[string]Target {
	m := make(map[string]Target, len(targetStrings))
	for i, name := range targetStrings {
		m[name] = Target(i)
	}
	return m
}()

// String returns t's canonical XML spelling.
func (t Target) String() string {
	if int(t) < len(targetStrings) {
		return targetStrings[t]
	}
	return fmt.Sprintf("Target(%d)", uint8(t))
}

// Element classifies the elemental affinity a skill attacks or defends with.
type Element uint8

const (
	ElementNone Element = iota
	ElementWind
	ElementFire
	ElementWater
	ElementEarth
	ElementHoly
	ElementDark
	ElementValakas
)

var elementStrings = [...]string{"NONE", "WIND", "FIRE", "WATER", "EARTH", "HOLY", "DARK", "VALAKAS"}

var elementNames = func() map[string]Element {
	m := make(map[string]Element, len(elementStrings))
	for i, name := range elementStrings {
		m[name] = Element(i)
	}
	return m
}()

// String returns e's canonical XML spelling.
func (e Element) String() string {
	if int(e) < len(elementStrings) {
		return elementStrings[e]
	}
	return fmt.Sprintf("Element(%d)", uint8(e))
}

// Flight classifies a forced-movement skill's trajectory.
type Flight uint8

const (
	FlightThrowUp Flight = iota
	FlightThrowHorizontal
	FlightDummy
	FlightCharge
)

var flightStrings = [...]string{"THROW_UP", "THROW_HORIZONTAL", "DUMMY", "CHARGE"}

var flightNames = func() map[string]Flight {
	m := make(map[string]Flight, len(flightStrings))
	for i, name := range flightStrings {
		m[name] = Flight(i)
	}
	return m
}()

// String returns f's canonical XML spelling.
func (f Flight) String() string {
	if int(f) < len(flightStrings) {
		return flightStrings[f]
	}
	return fmt.Sprintf("Flight(%d)", uint8(f))
}

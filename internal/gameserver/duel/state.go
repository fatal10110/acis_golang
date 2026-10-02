// Package duel owns the duels players fight against each other, alone or
// party against party: who stands in which duel, the countdown, the checks
// that end a duel and its result. It holds no persistent state and sends
// nothing itself: the network layer applies each Step a duel takes.
package duel

import "time"

// State is a player's standing in its duel.
type State int32

// Duel states.
const (
	NoDuel State = iota
	// Countdown: the duel is agreed and counting down to its start.
	Countdown
	// Duelling: the duel is under way and the player still fights.
	Duelling
	// Dead: the player lost, by defeat or surrender.
	Dead
	// Winner: the player's side won.
	Winner
	// Interrupted: something outside the duel touched the player; the duel
	// is cancelled on its next check.
	Interrupted
)

// Team is the team colour a duelling player shows, as the client numbers
// it.
type Team int32

// Teams.
const (
	TeamNone Team = 0
	TeamBlue Team = 1
	TeamRed  Team = 2
)

// Result is how a duel ended.
type Result uint8

// Results. Team 1 is the challenger's side, team 2 the challenged one's.
const (
	Continue Result = iota
	Team1Win
	Team2Win
	Team1Surrender
	Team2Surrender
	Canceled
	Timeout
)

const (
	// Range is how close two players must stand to challenge each other,
	// and how close a duel keeps its sides.
	Range = 2000
	// Length is how long a duel lasts before it ends in a tie.
	Length = 2 * time.Minute
	// TickPeriod is how often a duel counts down and checks its end
	// conditions.
	TickPeriod = time.Second

	soloCountdown  = 5
	partyCountdown = 35
	// teleportCountdown is the countdown second a party duel moves both
	// parties to the arena.
	teleportCountdown = 33
)

// Arena is where a party duel takes place: the challenger's party stands
// 150 units below it on the Y axis, the challenged party 150 above, each
// member 40 units further along X than the one before, starting 180 short
// of it.
var Arena = struct{ X, Y, Z int }{X: -83760, Y: -238825, Z: -3331}

// Standing is a player's duel standing as other rules read it.
type Standing interface {
	DuelID() int32
	DuelState() State
}

// SameActive reports whether a and b fight each other in the same duel:
// both are still duelling, in one duel.
func SameActive(a, b Standing) bool {
	return a.DuelState() == Duelling && b.DuelState() == Duelling && a.DuelID() == b.DuelID()
}

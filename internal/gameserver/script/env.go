package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// Env is the server state the script helpers act through. Build hands the
// same Env to every registered script; nothing changes it afterwards.
type Env struct {
	// Quests writes the quest journals whose states the checks return.
	Quests *Quests
	// Rates scale quest drops and rewards.
	Rates Rates
	// PartyRange is the distance a player must stand strictly within, from
	// the NPC, to pass a quest-state check.
	PartyRange int
	// MultipleItemDrop gives a non-stackable item counted above one as that
	// many instances; unset, as one.
	MultipleItemDrop bool
	// NewItemID allocates an item instance's object id.
	NewItemID func() (int32, error)
	// Rand returns a uniform random int in [0, n). It panics when n is not
	// positive. Every random draw of a script and its helpers is made
	// through it.
	Rand func(n int) int
}

// Rates are the quest drop and reward multipliers.
type Rates struct {
	// Drop scales quest drops, per drop type.
	Drop float64
	// Reward scales items rewarded, Adena scales adena rewarded.
	Reward, RewardAdena float64
	// XP and SP scale rewarded experience and skill points.
	XP, SP float64
}

// character resolves p to the character the world tracks. A handle on
// nothing panics, so the script's invocation aborts there.
func (p *Player) character() *player.Character {
	return p.self.(player.CharacterHolder).PlayerCharacter()
}

// Rnd returns a uniform random int in [0, n); it panics when n is not
// positive.
func (s *Script) Rnd(n int) int { return s.env.Rand(n) }

// RndBool returns true or false with even odds.
func (s *Script) RndBool() bool { return s.env.Rand(2) == 0 }

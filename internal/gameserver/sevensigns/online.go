package sevensigns

// Online is the players online as a period change acts on them beyond its
// notices. Its methods are called without the state's lock held, and each
// returns once every player it acts on has been dealt with.
type Online interface {
	// GiveStrifeSkills gives every player online the Seal of Strife skill
	// StrifeSkill names, as seal validation begins.
	GiveStrifeSkills()
	// RemoveStrifeSkills takes both Seal of Strife skills from every
	// player online, as seal validation ends.
	RemoveStrifeSkills()
	// ExpelFromDungeons sends every player online standing in a Seven
	// Signs dungeon, game masters aside, whom ExpelledAtPeriodChange names
	// to the nearest town and out of the dungeon. It runs after every
	// period change's save.
	ExpelFromDungeons()
}

// SetOnline hands the period changes' effects on the players online to
// online.
func (s *State) SetOnline(online Online) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.online = online
}

// StrifeSkill is the Seal of Strife skill a player holds during seal
// validation.
type StrifeSkill int

const (
	// NoStrifeSkill is a player in no cabal.
	NoStrifeSkill StrifeSkill = iota
	// VictorOfWar is the skill of a player whose cabal owns the seal.
	VictorOfWar
	// VanquishedOfWar is the skill of every other player in a cabal.
	VanquishedOfWar
)

// StrifeSkill returns the Seal of Strife skill objectID gets as seal
// validation begins: none outside a cabal, The Victor of War when its cabal
// owns the Seal of Strife, The Vanquished of War otherwise — also when no
// cabal owns the seal.
func (s *State) StrifeSkill(objectID int32) StrifeSkill {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.strifeSkillLocked(objectID)
}

// EnterStrifeSkill returns the Seal of Strife skill objectID gets entering
// the world. ok is false outside seal validation, or when no cabal owns
// the seal: the player then loses both skills instead.
func (s *State) EnterStrifeSkill(objectID int32) (skill StrifeSkill, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.row.Period != SealValidation || s.strifeOwnerLocked() == NoCabal {
		return NoStrifeSkill, false
	}
	return s.strifeSkillLocked(objectID), true
}

func (s *State) strifeSkillLocked(objectID int32) StrifeSkill {
	cabal := s.playerCabalLocked(objectID)
	switch {
	case cabal == NoCabal:
		return NoStrifeSkill
	case cabal == s.strifeOwnerLocked():
		return VictorOfWar
	default:
		return VanquishedOfWar
	}
}

func (s *State) strifeOwnerLocked() Cabal {
	i, _ := sealIndex(Strife)
	return s.row.SealOwners[i]
}

func (s *State) playerCabalLocked(objectID int32) Cabal {
	if p, ok := s.players[objectID]; ok {
		return p.Cabal
	}
	return NoCabal
}

// ExpelledAtPeriodChange reports whether objectID, standing in a Seven
// Signs dungeon as a period change completes, must leave it. A player who
// never signed up always leaves. During results and seal validation, one
// outside the winning cabal leaves; in the other periods one signed up for
// a cabal leaves, while a sign-up left without one stays — the reverse of
// the login rule, as the reference has it.
func (s *State) ExpelledAtPeriodChange(objectID int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.players[objectID]
	if !ok {
		return true
	}
	if s.row.Period == Results || s.row.Period == SealValidation {
		return p.Cabal != s.winningCabalLocked()
	}
	return p.Cabal != NoCabal
}

// ExpelledAtLogin reports whether objectID, entering the world in a Seven
// Signs dungeon, must leave it: during results and seal validation when it
// is outside the winning cabal, in the other periods when it is in no
// cabal. A player who never signed up counts as in no cabal.
func (s *State) ExpelledAtLogin(objectID int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cabal := s.playerCabalLocked(objectID)
	if s.row.Period == Results || s.row.Period == SealValidation {
		return cabal != s.winningCabalLocked()
	}
	return cabal == NoCabal
}

package sevensigns

import (
	"context"
	"fmt"
)

// PlayerCabal returns the cabal objectID signed up for, NoCabal when none.
func (s *State) PlayerCabal(objectID int32) Cabal {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.players[objectID]; ok {
		return p.Cabal
	}
	return NoCabal
}

// PlayerSeal returns the seal objectID chose, NoSeal when none.
func (s *State) PlayerSeal(objectID int32) Seal {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.players[objectID]; ok {
		return p.Seal
	}
	return NoSeal
}

// SetPlayerInfo signs objectID up for cabal choosing seal, and counts the
// choice toward the seal's votes; a Dawn sign-up counts for Dawn, anything
// else for Dusk. A player signing up for the first time is written to the
// store at once, with no stones turned in; an existing sign-up changes in
// memory and is written with the next save.
func (s *State) SetPlayerInfo(ctx context.Context, objectID int32, cabal Cabal, seal Seal) error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	s.mu.Lock()
	p, ok := s.players[objectID]
	if ok {
		p.Cabal, p.Seal = cabal, seal
	} else {
		p = &PlayerRow{ObjectID: objectID, Cabal: cabal, Seal: seal}
		s.players[objectID] = p
	}
	inserted := *p
	if i, valid := sealIndex(seal); valid {
		if cabal == Dawn {
			s.row.DawnSealVotes[i]++
		} else {
			s.row.DuskSealVotes[i]++
		}
	}
	s.mu.Unlock()

	if ok {
		return nil
	}
	if err := s.store.InsertPlayer(ctx, inserted); err != nil {
		return fmt.Errorf("insert seven signs player %d: %w", objectID, err)
	}
	return nil
}

// AddPlayerStoneContrib turns in objectID's stones by color: their points
// join the player's contribution, the ancient adena the player can collect,
// and the stone score of the player's cabal. It returns the points the
// stones were worth; ok is false, and nothing changes, when the player has
// not signed up or the contribution would pass maxContrib.
func (s *State) AddPlayerStoneContrib(objectID int32, blue, green, red, maxContrib int) (points int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, found := s.players[objectID]
	if !found {
		return 0, false
	}
	points = StoneScore(blue, green, red)
	contrib := p.ContributionScore + points
	if contrib > maxContrib {
		return 0, false
	}
	p.RedStones += red
	p.GreenStones += green
	p.BlueStones += blue
	p.AncientAdena += points
	p.ContributionScore = contrib
	switch p.Cabal {
	case Dawn:
		s.row.DawnStoneScore += float64(points)
	case Dusk:
		s.row.DuskStoneScore += float64(points)
	}
	return points, true
}

// TakeAncientAdenaReward returns the ancient adena objectID can collect and
// clears it together with the stones turned in. A player who has not signed
// up has nothing to collect.
func (s *State) TakeAncientAdenaReward(objectID int32) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.players[objectID]
	if !ok {
		return 0
	}
	reward := p.AncientAdena
	p.RedStones, p.GreenStones, p.BlueStones, p.AncientAdena = 0, 0, 0, 0
	return reward
}

// AddFestivalScore adds amount festival points to cabal (Dusk, anything
// else to Dawn) and takes the same amount from its rival, unless the rival
// has fewer points than that.
func (s *State) AddFestivalScore(cabal Cabal, amount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	gain, lose := &s.row.DawnFestivalScore, &s.row.DuskFestivalScore
	if cabal == Dusk {
		gain, lose = lose, gain
	}
	*gain += amount
	if *lose >= amount {
		*lose -= amount
	}
}

// Sky returns whose sky the world shows: the winning cabal's during seal
// validation, otherwise the regular one (NoCabal).
func (s *State) Sky() Cabal {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.row.Period != SealValidation {
		return NoCabal
	}
	return s.winningCabalLocked()
}

func (s *State) winningCabalLocked() Cabal {
	dawn, dusk := s.scoreLocked(Dawn), s.scoreLocked(Dusk)
	switch {
	case dawn == dusk:
		return NoCabal
	case dusk > dawn:
		return Dusk
	default:
		return Dawn
	}
}

// scoreLocked is cabal's total score.
func (s *State) scoreLocked(cabal Cabal) int {
	total := s.row.DawnStoneScore + s.row.DuskStoneScore
	switch cabal {
	case Dawn:
		return cabalScore(s.row.DawnStoneScore, total, s.row.DawnFestivalScore)
	case Dusk:
		return cabalScore(s.row.DuskStoneScore, total, s.row.DuskFestivalScore)
	}
	return 0
}

func (s *State) totalMembersLocked(cabal Cabal) int {
	n := 0
	for _, p := range s.players {
		if p.Cabal == cabal {
			n++
		}
	}
	return n
}

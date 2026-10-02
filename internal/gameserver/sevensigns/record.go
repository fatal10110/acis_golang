package sevensigns

// Record is the Record of Seven Signs as one player sees it: the calendar,
// the player's own sign-up, both cabals' standings, and the seals.
type Record struct {
	Period Period
	Cycle  int

	PlayerCabal Cabal
	PlayerSeal  Seal
	// PlayerStones counts the stones the player turned in this cycle.
	PlayerStones int
	// PlayerAncientAdena is what the player's stones are worth to collect.
	PlayerAncientAdena int

	Dusk CabalStanding
	Dawn CabalStanding
	// Winner is the cabal ahead on total score, NoCabal on a tie.
	Winner Cabal
	// Seals holds the three seals in Seals order.
	Seals [3]SealStanding
}

// CabalStanding is one cabal's scores.
type CabalStanding struct {
	// StoneProportion is the cabal's share of the stone pot scaled to 500.
	StoneProportion int
	FestivalScore   int
	TotalScore      int
	// Percent is the cabal's share of both cabals' total scores.
	Percent int
}

// SealStanding is one seal's ownership and votes.
type SealStanding struct {
	Seal  Seal
	Owner Cabal
	// DuskPercent and DawnPercent are the percent of each cabal's members
	// who chose the seal, zero for a cabal with no members.
	DuskPercent int
	DawnPercent int
	// Predicted is who would own the seal were the competition settled
	// now, and Prediction the reason.
	Predicted  Cabal
	Prediction Prediction
}

// Record returns the Record of Seven Signs for objectID.
func (s *State) Record(objectID int32) Record {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := Record{Period: s.row.Period, Cycle: s.row.Cycle, Winner: s.winningCabalLocked()}
	if p, ok := s.players[objectID]; ok {
		r.PlayerCabal, r.PlayerSeal = p.Cabal, p.Seal
		r.PlayerStones = p.RedStones + p.GreenStones + p.BlueStones
		r.PlayerAncientAdena = p.AncientAdena
	}

	totalStone := s.row.DuskStoneScore + s.row.DawnStoneScore
	r.Dusk = CabalStanding{
		StoneProportion: stoneProportion(s.row.DuskStoneScore, totalStone),
		FestivalScore:   s.row.DuskFestivalScore,
		TotalScore:      s.scoreLocked(Dusk),
	}
	r.Dawn = CabalStanding{
		StoneProportion: stoneProportion(s.row.DawnStoneScore, totalStone),
		FestivalScore:   s.row.DawnFestivalScore,
		TotalScore:      s.scoreLocked(Dawn),
	}
	if total := r.Dusk.TotalScore + r.Dawn.TotalScore; total != 0 {
		r.Dusk.Percent = share(r.Dusk.TotalScore, total, 100)
		r.Dawn.Percent = share(r.Dawn.TotalScore, total, 100)
	}

	dawnMembers, duskMembers := s.memberCountsLocked()
	for i, seal := range Seals {
		st := SealStanding{Seal: seal, Owner: s.row.SealOwners[i]}
		if duskMembers != 0 {
			st.DuskPercent = share(s.row.DuskSealVotes[i], duskMembers, 100)
		}
		if dawnMembers != 0 {
			st.DawnPercent = share(s.row.DawnSealVotes[i], dawnMembers, 100)
		}
		dawn, dusk := s.sealPercentsLocked(i, dawnMembers, duskMembers)
		st.Predicted, st.Prediction = sealOutcome(st.Owner, r.Winner, dawn, dusk)
		r.Seals[i] = st
	}
	return r
}

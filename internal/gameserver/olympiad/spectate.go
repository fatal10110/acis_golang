package olympiad

// Phase is how a stadium's match stands to whoever looks over the
// stadiums.
type Phase int

const (
	// PhaseInitial: the stadium has not brought a match's competitors in.
	PhaseInitial Phase = iota
	// PhaseCounting: the fight is counting down.
	PhaseCounting
	// PhaseInProgress: the competitors are fighting.
	PhaseInProgress
	// PhaseTerminate: the competitors are in, neither counting down nor
	// fighting.
	PhaseTerminate
)

// Stadiums are the Olympiad's stadiums, in order; none before
// BindStadiums.
func (o *Olympiad) Stadiums() []*Stadium {
	if o.games == nil {
		return nil
	}
	return o.games.stadiums
}

// Phase reports how the stadium's match stands and, past PhaseInitial,
// the names of its two competitors, side 1 first, read at one moment.
func (s *Stadium) Phase() (Phase, [2]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.game == nil || s.stage < stageGameStarted || s.stage > stageCleanup {
		return PhaseInitial, [2]string{}
	}
	names := [2]string{s.game.players[0].Name, s.game.players[1].Name}
	switch s.stage {
	case stageBattleCountdown:
		return PhaseCounting, names
	case stageBattleInProgress:
		return PhaseInProgress, names
	default:
		return PhaseTerminate, names
	}
}

package olympiad

// Noble is one noble's record for the running Olympiad cycle, as the
// olympiad_nobles table stores it. Name is the character's current name,
// read along with the record and never stored with it.
type Noble struct {
	ClassID   int
	Name      string
	Points    int
	CompDone  int
	CompWon   int
	CompLost  int
	CompDrawn int
	// Rewarded is set once the noble has traded this cycle's points for
	// noblesse passes.
	Rewarded bool
}

// addPoints adds amount, which may be negative, to n's points; the total
// never drops below zero.
func (n *Noble) addPoints(amount int) {
	n.Points = max(0, n.Points+amount)
}

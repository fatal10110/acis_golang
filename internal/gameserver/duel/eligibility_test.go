package duel

import "testing"

// healthy is a candidate no check refuses.
func healthy() Candidate {
	return Candidate{CurrentHP: 100, MaxHP: 100, CurrentMP: 50, MaxMP: 50}
}

func TestCanDuel(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Candidate)
		want Refusal
	}{
		{name: "healthy", set: func(*Candidate) {}, want: Eligible},
		{name: "exactly half hp and mp", set: func(c *Candidate) { c.CurrentHP, c.CurrentMP = 50, 25 }, want: Eligible},
		{name: "in combat", set: func(c *Candidate) { c.InCombat = true }, want: RefusedInBattle},
		{name: "jailed", set: func(c *Candidate) { c.Jailed = true }, want: RefusedInBattle},
		{name: "alike dead", set: func(c *Candidate) { c.AlikeDead = true }, want: RefusedHPOrMPBelowHalf},
		{name: "hp below half", set: func(c *Candidate) { c.CurrentHP = 49.9 }, want: RefusedHPOrMPBelowHalf},
		{name: "mp below half", set: func(c *Candidate) { c.CurrentMP = 24.9 }, want: RefusedHPOrMPBelowHalf},
		{name: "in duel", set: func(c *Candidate) { c.InDuel = true }, want: RefusedAlreadyDuelling},
		{name: "olympiad", set: func(c *Candidate) { c.InOlympiad = true }, want: RefusedOlympiad},
		{name: "chaotic", set: func(c *Candidate) { c.Chaotic = true }, want: RefusedChaotic},
		{name: "private store", set: func(c *Candidate) { c.Operating = true }, want: RefusedPrivateStore},
		{name: "riding", set: func(c *Candidate) { c.Riding = true }, want: RefusedRiding},
		{name: "fishing", set: func(c *Candidate) { c.Fishing = true }, want: RefusedFishing},
		{name: "prohibited area", set: func(c *Candidate) { c.ProhibitedArea = true }, want: RefusedProhibitedArea},
		{name: "every check fails: combat wins", set: func(c *Candidate) {
			*c = Candidate{InCombat: true, AlikeDead: true, InDuel: true, InOlympiad: true, Chaotic: true, Operating: true, Riding: true, Fishing: true, ProhibitedArea: true}
		}, want: RefusedInBattle},
		{name: "low hp before duelling", set: func(c *Candidate) { c.CurrentHP, c.InDuel = 1, true }, want: RefusedHPOrMPBelowHalf},
		{name: "duelling before olympiad", set: func(c *Candidate) { c.InDuel, c.InOlympiad = true, true }, want: RefusedAlreadyDuelling},
		{name: "olympiad before chaotic", set: func(c *Candidate) { c.InOlympiad, c.Chaotic = true, true }, want: RefusedOlympiad},
		{name: "chaotic before store", set: func(c *Candidate) { c.Chaotic, c.Operating = true, true }, want: RefusedChaotic},
		{name: "store before riding", set: func(c *Candidate) { c.Operating, c.Riding = true, true }, want: RefusedPrivateStore},
		{name: "riding before fishing", set: func(c *Candidate) { c.Riding, c.Fishing = true, true }, want: RefusedRiding},
		{name: "fishing before area", set: func(c *Candidate) { c.Fishing, c.ProhibitedArea = true, true }, want: RefusedFishing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := healthy()
			tt.set(&c)
			if got := CanDuel(c); got != tt.want {
				t.Fatalf("CanDuel = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestInRange(t *testing.T) {
	a := newFake(1, 0)
	if !InRange(a, newFake(2, Range-1)) {
		t.Fatal("a player just inside Range is out of range")
	}
	if InRange(a, newFake(2, Range)) {
		t.Fatal("a player at Range is in range")
	}
}

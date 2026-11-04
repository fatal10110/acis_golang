package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/vitals"
)

// probeDeathTable is the growth table of one probed death: the rows below,
// at and above the level, the last of which ends the table.
func probeDeathTable(t *testing.T, c vitals.PlayerDeathCase) *LevelTable {
	t.Helper()
	levels := map[int]Level{
		c.Level - 1: {RequiredExpToLevelUp: c.Prev},
		c.Level:     {RequiredExpToLevelUp: c.Cur, KarmaModifier: c.KarmaModifier, ExpLossAtDeath: c.LossPercent},
	}
	if c.Level < 81 {
		levels[c.Level+1] = Level{RequiredExpToLevelUp: c.Next}
	}
	table, err := NewLevelTable(levels)
	if err != nil {
		t.Fatalf("NewLevelTable() error = %v", err)
	}
	return table
}

// TestDeathPenaltyMatchesReferenceProbe checks the experience a death takes,
// the snapshot it keeps and the karma it costs against the reference's own
// answers (vitals.golden): the halved loss at war, a karma rate other than
// one, a loss that would go below zero, and the loss at the highest level.
func TestDeathPenaltyMatchesReferenceProbe(t *testing.T) {
	band := vitals.PlayerDeathCase{Level: 5, LossPercent: 10, KarmaModifier: 2, KarmaRate: 1, Karma: 240, Prev: 3, Cur: 1000, Next: 3000, Exp: 1500}
	with := func(edit func(*vitals.PlayerDeathCase)) vitals.PlayerDeathCase {
		c := band
		edit(&c)
		return c
	}
	cases := []vitals.PlayerDeathCase{
		band,
		with(func(c *vitals.PlayerDeathCase) { c.Karma = 0 }),
		with(func(c *vitals.PlayerDeathCase) { c.AtWar = true }),
		with(func(c *vitals.PlayerDeathCase) { c.AtWar, c.Karma, c.Exp = true, 0, 1100 }),
		with(func(c *vitals.PlayerDeathCase) { c.KarmaRate = 2 }),
		with(func(c *vitals.PlayerDeathCase) { c.KarmaRate = 0.5 }),
		with(func(c *vitals.PlayerDeathCase) { c.LossPercent = 100 }),
		with(func(c *vitals.PlayerDeathCase) { c.LossPercent, c.Exp = 100, 2500 }),
		{Level: 40, LossPercent: 4, KarmaModifier: 11.5, KarmaRate: 1, Karma: 1000, Prev: 4800000, Cur: 5000000, Next: 5400000, Exp: 5100000},
		{Level: 81, LossPercent: 1.5, KarmaModifier: 11.5, KarmaRate: 1, Karma: 1000, Prev: 2000000000, Cur: 2100000000, Next: 2147483647, Exp: 2100000000},
	}
	for _, in := range cases {
		want := vitals.PlayerDeath(t, in)
		c := &Character{ID: 1, CharLevel: in.Level, Exp: in.Exp, KarmaPoints: in.Karma}
		c.levelTable = probeDeathTable(t, in)
		var hooks progressionHooks
		c.progressionMu.Lock()
		c.applyDeathPenaltyLocked(deathLoss{table: c.levelTable, rate: in.KarmaRate, reducedLoss: in.AtWar}, &hooks)
		c.progressionMu.Unlock()
		if lost := in.Exp - c.Exp; lost != want.Lost || c.ExpBeforeDeath != want.Before || c.Exp != want.After || c.KarmaPoints != want.Karma {
			t.Errorf("%+v: lost %d, before %d, after %d, karma %d; reference %+v", in, lost, c.ExpBeforeDeath, c.Exp, c.KarmaPoints, want)
		}
	}
}

// TestRestoreExpMatchesReferenceProbe checks the experience a resurrection
// gives back against the reference's own answers (vitals.golden), including a
// share the reference narrows to an int and one that is negative because the
// character gained experience since its death.
func TestRestoreExpMatchesReferenceProbe(t *testing.T) {
	table, err := NewLevelTable(map[int]Level{1: {}, 2: {RequiredExpToLevelUp: 1 << 61}})
	if err != nil {
		t.Fatalf("NewLevelTable() error = %v", err)
	}
	for _, in := range []struct{ before, exp int64 }{
		{1500, 1300},
		{1500, 1500},
		{1500, 1505},
		{1500, 1503},
		{100000, 93160},
		{5000000000, 1000000000},
		{4000000000, 1000000},
		{3000000000, 1000000000},
	} {
		for _, power := range []float64{0, 50, 100} {
			want := vitals.PlayerExpRestored(t, in.before, in.exp, power)
			c := &Character{ID: 1, CharLevel: 1, Exp: in.exp, ExpBeforeDeath: in.before}
			c.levelTable = table
			c.RestoreExp(power)
			if got := c.Exp - in.exp; got != want {
				t.Errorf("RestoreExp(%v) from %d with %d before death added %d; reference %d", power, in.exp, in.before, got, want)
			}
		}
	}
}

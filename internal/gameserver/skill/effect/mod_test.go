package effect

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

func TestCalculatorOrdering(t *testing.T) {
	var c Calculator

	// Attach out of order; AddMod must still run them low-order-first.
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpAdd, Value: 5})     // order 30
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpMul, Value: 2})     // order 20
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpBaseAdd, Value: 3}) // order 2

	// base=10: BaseAdd -> 13, Mul -> 26, Add -> 31.
	got := c.Calc(nil, 10)
	if got != 31 {
		t.Errorf("Calc() = %v, want 31", got)
	}
	if c.Size() != 3 {
		t.Errorf("Size() = %d, want 3", c.Size())
	}
}

func TestCalculatorSetOverridesBase(t *testing.T) {
	var c Calculator
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpSet, Value: 100})
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpBaseMul, Value: 0.1})

	// Set (order 0) runs first, replacing base with 100 (value=100). Then
	// BaseMul (order 1) adds base*0.1 = 100*0.1 = 10 to the running value:
	// 100 + 10 = 110.
	got := c.Calc(nil, 5)
	if got != 110 {
		t.Errorf("Calc() = %v, want 110", got)
	}
}

func TestCalculatorRemoveOwner(t *testing.T) {
	var c Calculator
	ownerA := ModOwnerEffect(&Effect{})
	ownerB := ModOwnerEffect(&Effect{})
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpAdd, Value: 5, Owner: ownerA})
	c.AddMod(Mod{Stat: stat.MagicAttack, Op: OpAdd, Value: 7, Owner: ownerB})
	c.AddMod(Mod{Stat: stat.CriticalRate, Op: OpAdd, Value: 1, Owner: ownerA})

	c.RemoveOwner(ownerA)
	if c.Size() != 1 {
		t.Fatalf("Size() = %d, want 1", c.Size())
	}
	if got := c.Calc(nil, 0); got != 7 {
		t.Errorf("Calc() = %v, want 7 (only ownerB's mod left)", got)
	}
}

func TestCalculatorEmpty(t *testing.T) {
	var c Calculator
	if got := c.Calc(nil, 42); got != 42 {
		t.Errorf("Calc() on empty chain = %v, want base unchanged 42", got)
	}
}

func TestCalculatorBuiltinRunsBetweenLowAndHighOrderMods(t *testing.T) {
	c := NewCalculator(func(actor stat.Actor, base, value float64) float64 {
		return value + 1000
	})
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpBaseAdd, Value: 3}) // order 2, before builtin
	c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpAdd, Value: 5})     // order 30, after builtin

	// base=10: BaseAdd -> 13, builtin -> 1013, Add -> 1018.
	if got := c.Calc(nil, 10); got != 1018 {
		t.Errorf("Calc() = %v, want 1018", got)
	}
}

// TestCalculatorConcurrentReadsAndMutationsAreRace-Free exercises the
// concurrency guarantee #1527 replaced: many goroutines call Calc
// concurrently with other goroutines attaching and detaching Mods. This
// test's only purpose is to be run under -race; it makes no assertion
// about the numeric outcome, which is inherently nondeterministic under
// concurrent mutation.
func TestCalculatorConcurrentReadsAndMutationsAreRaceFree(t *testing.T) {
	var c Calculator
	owner := ModOwnerEffect(&Effect{})

	var readers, writers sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					c.Calc(nil, 10)
				}
			}
		}()
	}

	for i := 0; i < 2; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for j := 0; j < 200; j++ {
				c.AddMod(Mod{Stat: stat.PowerAttack, Op: OpAdd, Value: 1, Owner: owner})
				c.RemoveOwner(owner)
			}
		}()
	}

	writers.Wait()
	close(stop)
	readers.Wait()
}

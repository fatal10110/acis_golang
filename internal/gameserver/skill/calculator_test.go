package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

func TestCalculatorOrdering(t *testing.T) {
	var c Calculator

	// Attach out of order; AddFunc must still run them low-order-first.
	c.AddFunc(basefunc.NewAdd(nil, stat.PowerAttack, 5, nil))     // order 30
	c.AddFunc(basefunc.NewMul(nil, stat.PowerAttack, 2, nil))     // order 20
	c.AddFunc(basefunc.NewBaseAdd(nil, stat.PowerAttack, 3, nil)) // order 2

	// base=10: BaseAdd -> 13, Mul -> 26, Add -> 31.
	got := c.Calc(nil, nil, nil, 10)
	if got != 31 {
		t.Errorf("Calc() = %v, want 31", got)
	}
	if c.Size() != 3 {
		t.Errorf("Size() = %d, want 3", c.Size())
	}
}

func TestCalculatorSetOverridesBase(t *testing.T) {
	var c Calculator
	c.AddFunc(basefunc.NewSet(nil, stat.PowerAttack, 100, nil))
	c.AddFunc(basefunc.NewBaseMul(nil, stat.PowerAttack, 0.1, nil)) // order 1, before Set at order 0? no: Set is order 0, BaseMul is order 1.

	// Set (order 0) runs first, replacing base with 100 (value=100). Then
	// BaseMul (order 1) adds base*0.1 = 100*0.1 = 10 to the running value:
	// 100 + 10 = 110.
	got := c.Calc(nil, nil, nil, 5)
	if got != 110 {
		t.Errorf("Calc() = %v, want 110", got)
	}
}

func TestCalculatorRemoveFunc(t *testing.T) {
	var c Calculator
	fn := basefunc.NewAdd(nil, stat.PowerAttack, 5, nil)
	c.AddFunc(fn)
	c.AddFunc(basefunc.NewAdd(nil, stat.PowerAttack, 7, nil))

	c.RemoveFunc(fn)
	if c.Size() != 1 {
		t.Fatalf("Size() = %d, want 1", c.Size())
	}
	if got := c.Calc(nil, nil, nil, 0); got != 7 {
		t.Errorf("Calc() = %v, want 7", got)
	}
}

func TestCalculatorRemoveOwner(t *testing.T) {
	var c Calculator
	ownerA, ownerB := "a", "b"
	c.AddFunc(basefunc.NewAdd(ownerA, stat.PowerAttack, 5, nil))
	c.AddFunc(basefunc.NewAdd(ownerB, stat.MagicAttack, 7, nil))
	c.AddFunc(basefunc.NewAdd(ownerA, stat.CriticalRate, 1, nil))

	modified := c.RemoveOwner(ownerA)
	if len(modified) != 2 {
		t.Fatalf("RemoveOwner() removed %d funcs, want 2", len(modified))
	}
	if c.Size() != 1 {
		t.Fatalf("Size() = %d, want 1", c.Size())
	}
	if got := c.Calc(nil, nil, nil, 0); got != 7 {
		t.Errorf("Calc() = %v, want 7 (only ownerB's func left)", got)
	}
}

func TestCalculatorEmpty(t *testing.T) {
	var c Calculator
	if got := c.Calc(nil, nil, nil, 42); got != 42 {
		t.Errorf("Calc() on empty chain = %v, want base unchanged 42", got)
	}
}

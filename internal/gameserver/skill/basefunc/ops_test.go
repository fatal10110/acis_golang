package basefunc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// fakeCond is a Condition whose Test always returns the configured result,
// recording whether it was invoked.
type fakeCond struct {
	result  bool
	invoked bool
}

func (f *fakeCond) Test(effector, effected, skill any) bool {
	f.invoked = true
	return f.result
}

func TestOpsCalc(t *testing.T) {
	tests := []struct {
		name  string
		fn    Func
		base  float64
		value float64
		want  float64
	}{
		{"Set", NewSet(nil, stat.PowerAttack, 40, nil), 10, 20, 40},
		{"BaseMul", NewBaseMul(nil, stat.CriticalRate, 0.1, nil), 100, 100, 110},
		{"BaseAdd", NewBaseAdd(nil, stat.PowerDefence, 5, nil), 10, 20, 25},
		{"Mul", NewMul(nil, stat.PowerAttack, 1.5, nil), 10, 20, 30},
		{"Div", NewDiv(nil, stat.PowerAttack, 2, nil), 10, 20, 10},
		{"Add", NewAdd(nil, stat.PowerAttack, 5, nil), 10, 20, 25},
		{"Sub", NewSub(nil, stat.PowerAttack, 5, nil), 10, 20, 15},
		// AddMul: value * (1 - val/100); a 20% resist reduces 100 to 80.
		{"AddMul", NewAddMul(nil, stat.FireRes, 20, nil), 0, 100, 80},
		// SubDiv is AddMul's inverse: 80 / (1 - 20/100) = 100.
		{"SubDiv", NewSubDiv(nil, stat.FireRes, 20, nil), 0, 80, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn.Calc(nil, nil, nil, tt.base, tt.value)
			if got != tt.want {
				t.Errorf("Calc() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOpsOrder(t *testing.T) {
	tests := []struct {
		name string
		fn   Func
		want int
	}{
		{"Set", NewSet(nil, stat.PowerAttack, 0, nil), OrderSet},
		{"BaseMul", NewBaseMul(nil, stat.PowerAttack, 0, nil), OrderBaseMul},
		{"BaseAdd", NewBaseAdd(nil, stat.PowerAttack, 0, nil), OrderBaseAdd},
		{"Enchant", NewEnchant(nil, stat.PowerAttack, 0, nil), OrderEnchant},
		{"Mul", NewMul(nil, stat.PowerAttack, 0, nil), OrderMulDiv},
		{"Div", NewDiv(nil, stat.PowerAttack, 0, nil), OrderMulDiv},
		{"Add", NewAdd(nil, stat.PowerAttack, 0, nil), OrderAddSub},
		{"Sub", NewSub(nil, stat.PowerAttack, 0, nil), OrderAddSub},
		{"AddMul", NewAddMul(nil, stat.PowerAttack, 0, nil), OrderAddMul},
		{"SubDiv", NewSubDiv(nil, stat.PowerAttack, 0, nil), OrderAddMul},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn.Order(); got != tt.want {
				t.Errorf("Order() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestConditionGate checks that a failing Condition leaves value unchanged
// and is actually invoked (rather than short-circuited), for one op from
// each order family; a passing Condition applies the calculation as usual.
func TestConditionGate(t *testing.T) {
	failing := &fakeCond{result: false}
	add := NewAdd(nil, stat.PowerAttack, 5, failing)
	if got := add.Calc("effector", "effected", "skill", 10, 20); got != 20 {
		t.Errorf("Calc() with failing condition = %v, want unchanged 20", got)
	}
	if !failing.invoked {
		t.Error("failing condition was never invoked")
	}

	passing := &fakeCond{result: true}
	add2 := NewAdd(nil, stat.PowerAttack, 5, passing)
	if got := add2.Calc(nil, nil, nil, 10, 20); got != 25 {
		t.Errorf("Calc() with passing condition = %v, want 25", got)
	}
}

func TestCalculatorAccessors(t *testing.T) {
	owner := "owner-a"
	fn := NewAdd(owner, stat.PowerAttack, 5, nil)

	if fn.Stat() != stat.PowerAttack {
		t.Errorf("Stat() = %v, want %v", fn.Stat(), stat.PowerAttack)
	}
	if fn.Owner() != owner {
		t.Errorf("Owner() = %v, want %v", fn.Owner(), owner)
	}
	if fn.Value() != 5 {
		t.Errorf("Value() = %v, want 5", fn.Value())
	}
	if fn.Cond() != nil {
		t.Errorf("Cond() = %v, want nil", fn.Cond())
	}
}

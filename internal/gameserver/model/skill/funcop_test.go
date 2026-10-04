package skill

import "testing"

// TestParseFuncOpIgnoresCase checks that a stat-template tag matches its
// operation case-insensitively, as DocumentBase.parseTemplate's
// equalsIgnoreCase does (DocumentBase.java:145-166).
func TestParseFuncOpIgnoresCase(t *testing.T) {
	t.Parallel()
	cases := map[string]FuncOp{
		"add": FuncAdd, "Add": FuncAdd, "ADD": FuncAdd,
		"addMul": FuncAddMul, "ADDmul": FuncAddMul,
		"sub": FuncSub, "Sub": FuncSub,
		"subDiv": FuncSubDiv, "SUBDIV": FuncSubDiv,
		"mul": FuncMul, "mUl": FuncMul,
		"basemul": FuncBaseMul, "BaseMul": FuncBaseMul,
		"div": FuncDiv, "Div": FuncDiv,
		"set": FuncSet, "Set": FuncSet,
		"enchant": FuncEnchant, "Enchant": FuncEnchant,
		"baseadd": FuncBaseAdd, "BaseAdd": FuncBaseAdd,
	}
	for tag, want := range cases {
		got, err := ParseFuncOp(tag)
		if err != nil || got != want {
			t.Errorf("ParseFuncOp(%q) = %v, %v; want %v", tag, got, err, want)
		}
	}
	for _, tag := range []string{"", "adds", "cond", "effect", "base_add"} {
		if _, err := ParseFuncOp(tag); err == nil {
			t.Errorf("ParseFuncOp(%q) succeeded, want an error", tag)
		}
	}
}

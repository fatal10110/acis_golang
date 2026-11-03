package scenario

import "testing"

// A rows check lists vars whose names prefix each other (talk, talk1) in
// any order; the database returns them in its own collation's order.
func TestRowsDiffOrdersPrefixedVarsAlike(t *testing.T) {
	db := []string{"<cond>=10", "<state>=STARTED", "talk=1", "talk1=1", "talk2=0"}
	for _, want := range [][]string{
		{"<cond>=10", "<state>=STARTED", "talk=1", "talk1=1", "talk2=0"},
		{"talk2=0", "talk1=1", "talk=1", "<state>=STARTED", "<cond>=10"},
	} {
		if d := rowsDiff("Q120_PavelsResearch", db, want); d != "" {
			t.Fatalf("rows %q: %s", want, d)
		}
	}
	if d := rowsDiff("Q120_PavelsResearch", db, []string{"<cond>=10", "<state>=STARTED", "talk=1", "talk1=0", "talk2=0"}); d == "" {
		t.Fatal("a differing talk1 value passes")
	}
	if d := rowsDiff("Q120_PavelsResearch", nil, []string{"-"}); d != "" {
		t.Fatalf("no rows: %s", d)
	}
	if d := rowsDiff("Q120_PavelsResearch", db[:1], []string{"-"}); d == "" {
		t.Fatal("a row passes a rows - check")
	}
}

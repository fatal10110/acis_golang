package conditions

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func andClause() modelskill.ConditionClause {
	return modelskill.ConditionClause{Root: modelskill.Condition{
		Kind:     "and",
		Children: []modelskill.Condition{{Kind: "player", Attrs: map[string]string{"level": "10"}}},
	}}
}

// TestCompiledClauseBuildsOncePerClause proves a clause compiles once: every
// later test of the same clause reuses its tree, while another clause, even
// an equal one in the same slice, gets its own.
func TestCompiledClauseBuildsOncePerClause(t *testing.T) {
	t.Parallel()
	clauses := []modelskill.ConditionClause{andClause(), andClause()}
	first, err := CompiledClause(&clauses[0])
	if err != nil || first == nil {
		t.Fatalf("CompiledClause() = %v, %v; want a tree", first, err)
	}
	again, _ := CompiledClause(&clauses[0])
	if again != first {
		t.Fatal("second CompiledClause() of one clause built a new tree")
	}
	other, _ := CompiledClause(&clauses[1])
	if other == first {
		t.Fatal("CompiledClause() of another clause returned the first clause's tree")
	}
	if c, err := CompiledClause(nil); c != nil || err != nil {
		t.Fatalf("CompiledClause(nil) = %v, %v; want nil, nil", c, err)
	}
}

// TestCompiledKeepsCompileOutcome proves the cached result is Compile's: an
// error stays an error on every test, and a node holding no condition stays
// nil.
func TestCompiledKeepsCompileOutcome(t *testing.T) {
	t.Parallel()
	bad := modelskill.Condition{Kind: "skill"}
	for range 2 {
		if _, err := CompiledNode(&bad); err == nil {
			t.Fatal("CompiledNode(<skill> without stat) error = nil, want the compile error")
		}
	}
	empty := item.UseCondition{Root: item.Condition{Kind: "player"}}
	for range 2 {
		if c, err := CompiledItemUse(&empty); c != nil || err != nil {
			t.Fatalf("CompiledItemUse(no attribute) = %v, %v; want nil, nil", c, err)
		}
	}
	node := item.Condition{Kind: "player", Attrs: map[string]string{"level": "10"}}
	if c, err := CompiledItemNode(&node); c == nil || err != nil {
		t.Fatalf("CompiledItemNode() = %v, %v; want a tree", c, err)
	}
}

// TestCompiledClauseConcurrentFirstUse runs first tests of one clause from
// many goroutines; run with -race.
func TestCompiledClauseConcurrentFirstUse(t *testing.T) {
	t.Parallel()
	clause := andClause()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if c, err := CompiledClause(&clause); c == nil || err != nil {
				t.Errorf("CompiledClause() = %v, %v; want a tree", c, err)
			}
		})
	}
	wg.Wait()
}

func BenchmarkCompileClause(b *testing.B) {
	clause := andClause()
	b.Run("compile", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = Compile(clause.Root)
		}
	})
	b.Run("compiled", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = CompiledClause(&clause)
		}
	})
}

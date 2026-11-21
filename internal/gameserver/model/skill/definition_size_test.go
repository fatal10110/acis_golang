package skill

import (
	"testing"
	"unsafe"
)

// TestDefinitionSize pins the size of one stored level: a loaded table holds
// about 30,000 of them, so a field added to Definition rather than Extra
// shows up here.
func TestDefinitionSize(t *testing.T) {
	if got := unsafe.Sizeof(Definition{}); got != 296 {
		t.Fatalf("unsafe.Sizeof(Definition{}) = %d, want 296", got)
	}
}

// TestDefinitionExtraZeroAndCopy requires a Definition without an Extra to
// read the zero Extra, and WithExtra to leave its receiver as it was.
func TestDefinitionExtraZeroAndCopy(t *testing.T) {
	var bare Definition
	if got := bare.Extra(); got.NpcID != 0 || got.SelfEffects != nil || got.TeleCoords != nil {
		t.Fatalf("zero Definition Extra() = %+v, want the zero Extra", got)
	}
	with := bare.WithExtra(Extra{NpcID: 9})
	if with.Extra().NpcID != 9 || bare.Extra().NpcID != 0 {
		t.Fatalf("WithExtra NpcID: copy %d, receiver %d; want 9, 0", with.Extra().NpcID, bare.Extra().NpcID)
	}
	read := with.Extra()
	read.NpcID = 11
	if with.Extra().NpcID != 9 {
		t.Fatalf("writing the Extra() copy changed the Definition: NpcID = %d, want 9", with.Extra().NpcID)
	}
}

// TestTableSharesEqualExtra requires neighbouring stored levels with equal
// Extra values to hold one Extra, a level with different values to keep its
// own, and every level to read its own values back.
func TestTableSharesEqualExtra(t *testing.T) {
	table := NewTable([]Definition{
		NewDefinition(4, 1, "a", DefinitionAttrs{SkillType: "BUFF", Extra: Extra{NegateTypes: []string{"BUFF"}, NpcID: 5}}),
		NewDefinition(4, 2, "a", DefinitionAttrs{SkillType: "BUFF", Extra: Extra{NegateTypes: []string{"BUFF"}, NpcID: 5}}),
		NewDefinition(4, 3, "a", DefinitionAttrs{SkillType: "BUFF", Extra: Extra{NegateTypes: []string{"DEBUFF"}, NpcID: 5}}),
		NewDefinition(4, 4, "a", DefinitionAttrs{SkillType: "BUFF", Extra: Extra{NegateTypes: []string{"DEBUFF"}, NpcID: 5}}),
	})
	get := func(level int) *Definition {
		t.Helper()
		d, ok := table.Get(4, level)
		if !ok {
			t.Fatalf("Get(4, %d) missing", level)
		}
		return d
	}
	if get(1).extra != get(2).extra || get(3).extra != get(4).extra {
		t.Fatal("equal neighbouring Extra values are stored twice")
	}
	if get(2).extra == get(3).extra {
		t.Fatal("levels with different Extra values share one")
	}
	for level, want := range map[int]string{1: "BUFF", 2: "BUFF", 3: "DEBUFF", 4: "DEBUFF"} {
		if got := get(level).Extra().NegateTypes; len(got) != 1 || got[0] != want {
			t.Fatalf("level %d NegateTypes = %v, want [%s]", level, got, want)
		}
	}
}

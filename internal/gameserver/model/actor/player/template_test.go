package player

import (
	"sort"
	"testing"
)

func TestTemplateTable_All(t *testing.T) {
	// 0, 10 and 18 are base professions (classParent maps them to -1), so
	// NewTemplateTable needs no other entries to resolve them.
	table, err := NewTemplateTable(map[int]*Template{
		18: {ID: 18},
		0:  {ID: 0},
		10: {ID: 10},
	})
	if err != nil {
		t.Fatalf("NewTemplateTable() error: %v", err)
	}

	all := table.All()
	if len(all) != table.Count() {
		t.Fatalf("All() returned %d templates, Count() = %d", len(all), table.Count())
	}

	var ids []int
	for _, tpl := range all {
		ids = append(ids, tpl.ID)
	}
	if !sort.IntsAreSorted(ids) {
		t.Fatalf("All() not sorted ascending by ID: %v", ids)
	}
	if want := []int{0, 10, 18}; !equalInts(ids, want) {
		t.Fatalf("All() ids = %v, want %v", ids, want)
	}
}

func TestSkillGrantCorrectedCost(t *testing.T) {
	if got := (SkillGrant{Cost: -1}).CorrectedCost(); got != 0 {
		t.Fatalf("CorrectedCost(-1) = %d, want 0", got)
	}
	if got := (SkillGrant{Cost: 50}).CorrectedCost(); got != 50 {
		t.Fatalf("CorrectedCost(50) = %d, want 50", got)
	}
}

func TestTemplateSkillLearning(t *testing.T) {
	tmpl := &Template{Skills: []SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: 3, Level: 2, MinLevel: 5, Cost: 50},
		{SkillID: 3, Level: 3, MinLevel: 10, Cost: 370},
		{SkillID: 194, Level: 1, MinLevel: 1, Cost: 0},
		{SkillID: 1405, Level: 1, MinLevel: 5, Cost: -1},
	}}

	if got, ok := tmpl.FindSkillGrant(3, 2); !ok || got.Level != 2 {
		t.Fatalf("FindSkillGrant(3, 2) = %+v, %v; want level 2", got, ok)
	}
	if _, ok := tmpl.FindSkillGrant(3, 4); ok {
		t.Fatal("FindSkillGrant(3, 4) found a missing grant")
	}

	available := tmpl.AvailableSkillGrants(5, SkillLevels{3: 0})
	want := []SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: 1405, Level: 1, MinLevel: 5, Cost: -1},
	}
	if !equalSkillGrants(available, want) {
		t.Fatalf("AvailableSkillGrants(level 5, known none) = %+v, want %+v", available, want)
	}

	available = tmpl.AvailableSkillGrants(5, SkillLevels{3: 1, 1405: 1})
	want = []SkillGrant{{SkillID: 3, Level: 2, MinLevel: 5, Cost: 50}}
	if !equalSkillGrants(available, want) {
		t.Fatalf("AvailableSkillGrants(level 5, known 3:1) = %+v, want %+v", available, want)
	}

	if got := tmpl.RequiredLevelForNextSkillGrant(5); got != 10 {
		t.Fatalf("RequiredLevelForNextSkillGrant(level 5) = %d, want 10", got)
	}
	if got := tmpl.RequiredLevelForNextSkillGrant(10); got != 0 {
		t.Fatalf("RequiredLevelForNextSkillGrant(level 10) = %d, want 0", got)
	}

	grant, status := tmpl.CheckSkillLearn(5, 49, SkillLevels{}, 3, 1)
	if status != LearnNeedsSP || grant.CorrectedCost() != 50 {
		t.Fatalf("CheckSkillLearn(not enough SP) = %+v, %v; want cost 50 and LearnNeedsSP", grant, status)
	}

	grant, status = tmpl.CheckSkillLearn(5, 50, SkillLevels{}, 3, 1)
	if status != LearnAllowed || grant.SkillID != 3 || grant.Level != 1 {
		t.Fatalf("CheckSkillLearn(enough SP) = %+v, %v; want skill 3 level 1 and LearnAllowed", grant, status)
	}

	grant, status = tmpl.CheckSkillLearn(5, 0, SkillLevels{}, 1405, 1)
	if status != LearnAllowed || grant.CorrectedCost() != 0 {
		t.Fatalf("CheckSkillLearn(corrected zero cost) = %+v, %v; want allowed zero-cost grant", grant, status)
	}

	if _, status = tmpl.CheckSkillLearn(5, 1000, SkillLevels{3: 0}, 3, 2); status != LearnUnavailable {
		t.Fatalf("CheckSkillLearn(skipped previous level) = %v, want LearnUnavailable", status)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalSkillGrants(a, b []SkillGrant) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

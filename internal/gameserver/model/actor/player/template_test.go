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

// TestTemplateAutoGetSkillGrants pins which grants a level hands over for
// free: only an exact cost of 0, only the highest level unlocked per skill,
// and only where the character is not already at or above that level.
func TestTemplateAutoGetSkillGrants(t *testing.T) {
	tmpl := &Template{Skills: []SkillGrant{
		{SkillID: 194, Level: 1, MinLevel: 1, Cost: 0},
		{SkillID: 249, Level: 1, MinLevel: 5, Cost: 0},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
		{SkillID: 249, Level: 3, MinLevel: 20, Cost: 0},
		// Bought, and the -1 variant that merely displays a price of 0.
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: 1405, Level: 1, MinLevel: 5, Cost: -1},
	}}

	got := tmpl.AutoGetSkillGrants(10, SkillLevels{})
	want := []SkillGrant{
		{SkillID: 194, Level: 1, MinLevel: 1, Cost: 0},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
	}
	if !equalSkillGrants(got, want) {
		t.Fatalf("AutoGetSkillGrants(level 10, known none) = %+v, want %+v", got, want)
	}

	got = tmpl.AutoGetSkillGrants(10, SkillLevels{194: 1, 249: 2})
	if len(got) != 0 {
		t.Fatalf("AutoGetSkillGrants(level 10, already granted) = %+v, want none", got)
	}

	got = tmpl.AutoGetSkillGrants(10, SkillLevels{249: 1})
	want = []SkillGrant{
		{SkillID: 194, Level: 1, MinLevel: 1, Cost: 0},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
	}
	if !equalSkillGrants(got, want) {
		t.Fatalf("AutoGetSkillGrants(level 10, known 249:1) = %+v, want %+v", got, want)
	}
}

func TestTemplateAllAvailableSkillGrants(t *testing.T) {
	tmpl := &Template{Skills: []SkillGrant{
		{SkillID: 194, Level: 1, MinLevel: 1, Cost: 0},
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: 249, Level: 1, MinLevel: 5, Cost: 0},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
		{SkillID: 3, Level: 2, MinLevel: 10, Cost: -1},
		{SkillID: 249, Level: 3, MinLevel: 20, Cost: 0},
	}}

	got := tmpl.AllAvailableSkillGrants(10, SkillLevels{194: 1, 249: 1})
	want := []SkillGrant{
		{SkillID: 3, Level: 2, MinLevel: 10, Cost: -1},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
	}
	if !equalSkillGrants(got, want) {
		t.Fatalf("AllAvailableSkillGrants(level 10, known 194:1/249:1) = %+v, want %+v", got, want)
	}
}

// TestTemplateReachableSkillGrants pins the nine-level slack every skill but
// expertise keeps, so a small level loss does not strip skills the character
// legitimately learned.
func TestTemplateReachableSkillGrants(t *testing.T) {
	tmpl := &Template{Skills: []SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: 3, Level: 2, MinLevel: 20, Cost: 50},
		{SkillID: 239, Level: 1, MinLevel: 20, Cost: 0},
		{SkillID: 239, Level: 2, MinLevel: 40, Cost: 0},
	}}

	// At level 15 the lookahead reaches skill 3's level-20 grant, but
	// expertise stays pinned to the level itself and so has no grant yet.
	reachable := tmpl.ReachableSkillGrants(15)
	if got, ok := reachable[3]; !ok || got.Level != 2 {
		t.Fatalf("ReachableSkillGrants(15)[3] = %+v, %v; want level 2", got, ok)
	}
	if got, ok := reachable[239]; ok {
		t.Fatalf("ReachableSkillGrants(15)[239] = %+v; want no expertise grant", got)
	}

	if got, ok := tmpl.ReachableSkillGrants(20)[239]; !ok || got.Level != 1 {
		t.Fatalf("ReachableSkillGrants(20)[239] = %+v, %v; want level 1", got, ok)
	}
	if got, ok := tmpl.ReachableSkillGrants(40)[239]; !ok || got.Level != 2 {
		t.Fatalf("ReachableSkillGrants(40)[239] = %+v, %v; want level 2", got, ok)
	}

	// One level short of the lookahead, skill 3 falls back to its lower
	// grant rather than dropping out entirely.
	if got, ok := tmpl.ReachableSkillGrants(10)[3]; !ok || got.Level != 1 {
		t.Fatalf("ReachableSkillGrants(10)[3] = %+v, %v; want level 1", got, ok)
	}
	// Far enough below every grant, nothing is reachable at all.
	if reachable := (&Template{Skills: []SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 20, Cost: 50},
	}}).ReachableSkillGrants(10); len(reachable) != 0 {
		t.Fatalf("ReachableSkillGrants(10) = %+v, want none", reachable)
	}

	if !tmpl.GrantsSkill(3) {
		t.Error("GrantsSkill(3) = false, want true")
	}
	if tmpl.GrantsSkill(4267) {
		t.Error("GrantsSkill(4267) = true, want false for a skill the line never grants")
	}
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

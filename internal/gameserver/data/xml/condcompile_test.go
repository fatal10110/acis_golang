package xml

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestConditionThatDoesNotCompileIsLogged: load validation rejects every
// condition the evaluator cannot build, so this pins the load-time compile
// pass on its own: a clause that does not compile is logged once per
// skill level or item, and nothing is dropped.
func TestConditionThatDoesNotCompileIsLogged(t *testing.T) {
	t.Parallel()
	noStat := skill.Condition{Kind: "skill"} // a <skill> leaf needs a stat
	defs := []skill.Definition{
		{ID: 1, Level: 1, Conditions: []skill.ConditionClause{{Root: noStat}}},
		{ID: 1, Level: 2, Conditions: []skill.ConditionClause{{Root: skill.Condition{Kind: "player", Attrs: map[string]string{"level": "1"}}}}},
	}
	var logs bytes.Buffer
	compileSkillConditions(defs, zerolog.New(&logs))
	if got := strings.Count(logs.String(), "skill condition does not compile"); got != 1 {
		t.Errorf("skill compile diagnostics = %d, want 1; log = %s", got, logs.String())
	}

	logs.Reset()
	compileItemConditions([]*item.Template{
		{ID: 70, UseConditions: []item.UseCondition{{Root: item.Condition{Kind: "skill"}}}},
		{ID: 71, UseConditions: []item.UseCondition{{Root: item.Condition{Kind: "player", Attrs: map[string]string{"level": "1"}}}}},
	}, zerolog.New(&logs))
	if got := strings.Count(logs.String(), "item use condition does not compile"); got != 1 {
		t.Errorf("item compile diagnostics = %d, want 1; log = %s", got, logs.String())
	}
}

// TestShippedDataLoadsWithoutDiagnostics: the shipped skills, items, NPCs,
// spawnlist and doors load with no logged diagnostic: nothing skipped or
// partly loaded, no unresolved table value, and no condition that does not
// compile.
func TestShippedDataLoadsWithoutDiagnostics(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	log := zerolog.New(&logs).Level(zerolog.WarnLevel)
	xmlPath := func(name string) string { return datapackPath(t, filepath.Join("data", "xml", name)) }
	skills, err := LoadSkillDefinitions(xmlPath("skills"), log)
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	items, err := LoadItemTemplates(xmlPath("items"), log)
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}
	if _, err := LoadNPCTemplates(xmlPath("npcs"), items, skills, log); err != nil {
		t.Fatalf("LoadNPCTemplates: %v", err)
	}
	if _, err := LoadSpawnlist(xmlPath("spawnlist"), log, 1); err != nil {
		t.Fatalf("LoadSpawnlist: %v", err)
	}
	if _, err := LoadDoors(xmlPath("doors.xml"), log); err != nil {
		t.Fatalf("LoadDoors: %v", err)
	}
	if logs.Len() != 0 {
		t.Errorf("shipped data load logged diagnostics:\n%s", logs.String())
	}
}

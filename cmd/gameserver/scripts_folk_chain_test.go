package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// TestWarriorAggressiveChainRegistersOnFolk: the WarriorAggressive chain,
// bound to the civilian NPC 35592 the reference binds a child of it to,
// registers with the datapack's kind of that NPC and the catalog's handle
// operations, on the events the reference folds for it.
func TestWarriorAggressiveChainRegistersOnFolk(t *testing.T) {
	root := datapack.Path(t, "data", "xml")
	items, err := gamexml.LoadItemTemplates(filepath.Join(root, "items"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	skills, err := gamexml.LoadSkillDefinitions(filepath.Join(root, "skills"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	npcs, err := gamexml.LoadNPCTemplates(filepath.Join(root, "npcs"), items, skills, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	kindOf := npcKindOf(npcs)
	if k, ok := kindOf(35592); !ok || k != script.KindFolk {
		t.Fatalf("kind of 35592 = %v, %v; want a civilian NPC", k, ok)
	}

	const path = "ai.FolkWarriorAggressive"
	chain := func() script.Script {
		s := warrior.Aggressive()
		s.NPCs = []int32{35592}
		return s
	}
	ops := map[string]script.HandleOps{path: handleOps()["script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorAggressive"]}
	var logs bytes.Buffer
	r := script.Build([]script.Listing{{Path: path}}, script.Catalog{path: chain}, script.Config{KindOf: kindOf, Log: zerolog.New(&logs), HandleOps: ops})
	var dump bytes.Buffer
	if err := r.Dump(&dump); err != nil {
		t.Fatal(err)
	}
	want := "fold npc ATTACKED,ATTACK_FINISHED,CLAN_ATTACKED,CREATED,MY_DYING,NO_DESIRE,SEE_CREATURE,SEE_SPELL,USE_SKILL_FINISHED,SCRIPT_EVENT " + path + " 35592"
	if !strings.Contains(dump.String(), want+"\n") {
		t.Fatalf("dump misses %q:\n%s\nlog: %s", want, dump.String(), logs.String())
	}
}

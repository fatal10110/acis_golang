package main

import (
	"fmt"
	"path/filepath"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/rs/zerolog"
)

// scriptCatalogs are the literal script catalogs, one per range or family;
// scriptCatalog joins them.
func scriptCatalogs() []script.Catalog {
	return nil
}

// scriptCatalog joins catalogs into one. A path in two catalogs is an
// error.
func scriptCatalog(catalogs []script.Catalog) (script.Catalog, error) {
	out := script.Catalog{}
	for _, c := range catalogs {
		for path, ctor := range c {
			if _, dup := out[path]; dup {
				return nil, fmt.Errorf("script %s is in two catalogs", path)
			}
			out[path] = ctor
		}
	}
	return out, nil
}

// provideScripts builds the script registry from scripts.xml and the
// catalogs, against the NPC templates the boot loaded. Bindings and the
// seam gate are computed once, here, and the registry is never rebuilt:
// ids that had a template at boot keep their bindings across a later
// //reload npc, ids a reload adds stay unbound, and a template whose kind
// a reload changes is not re-checked by the seam gate until restart.
func provideScripts(paths gameServerPaths, data *gameData, log zerolog.Logger) (*script.Registry, error) {
	list, err := gamexml.LoadScriptList(filepath.Join(paths.DataRoot, "data", "xml", "scripts.xml"), log)
	if err != nil {
		return nil, err
	}
	catalog, err := scriptCatalog(scriptCatalogs())
	if err != nil {
		return nil, err
	}
	return script.Build(list, catalog, script.Config{KindOf: npcKindOf(data.NPCs), Log: log}), nil
}

// provideQuestJournals returns the quest journal writer, draining each
// player's journal writes on its persistence lane.
func provideQuestJournals(store *gamesql.QuestStore, worker *persist.Worker, log zerolog.Logger) *script.Quests {
	return script.NewQuests(store, worker, log)
}

// npcKindOf returns the kind each NPC template spawns as.
func npcKindOf(templates *npc.Table) func(int32) (script.NPCKind, bool) {
	return func(id int32) (script.NPCKind, bool) {
		tmpl, ok := templates.Get(int(id))
		if !ok {
			return script.KindOther, false
		}
		probe := &npc.Instance{Template: tmpl}
		switch {
		case npc.FolkKind(probe):
			return script.KindFolk, true
		case npc.Attackable(probe):
			return script.KindHostile, true
		default:
			return script.KindOther, true
		}
	}
}

// buildScripts makes the boot build the registry, so scripts.xml is read
// and every unported or refused script is reported, before the slices that
// raise hooks take it as a dependency.
func buildScripts(*script.Registry) {}

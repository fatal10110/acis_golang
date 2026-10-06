package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	scripttask "github.com/fatal10110/acis_golang/internal/gameserver/script/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// scriptCatalogs are the literal script catalogs, one per range or family;
// scriptCatalog joins them.
func scriptCatalogs() []script.Catalog {
	return []script.Catalog{questCatalog(), taskCatalog()}
}

// questCatalog lists the quests.
func questCatalog() script.Catalog {
	return script.Catalog{
		"quest.Q001_LettersOfLove": q001.New,
	}
}

// taskCatalog lists the scheduled tasks.
func taskCatalog() script.Catalog {
	return script.Catalog{
		"task.CastleTaxRefresh":     scripttask.CastleTaxRefresh,
		"task.ClanLadderRefresh":    scripttask.ClanLadderRefresh,
		"task.ClanLeaderTransfer":   scripttask.ClanLeaderTransfer,
		"task.RaidPointReset":       scripttask.RaidPointReset,
		"task.RecommendationUpdate": scripttask.RecommendationUpdate,
		"task.SevenSignsUpdate":     scripttask.SevenSignsUpdate,
	}
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
func provideScripts(paths gameServerPaths, data *gameData, env *script.Env, log zerolog.Logger) (*script.Registry, error) {
	list, err := gamexml.LoadScriptList(filepath.Join(paths.DataRoot, "data", "xml", "scripts.xml"), log)
	if err != nil {
		return nil, err
	}
	catalog, err := scriptCatalog(scriptCatalogs())
	if err != nil {
		return nil, err
	}
	return script.Build(list, catalog, script.Config{KindOf: npcKindOf(data.NPCs), Log: log, Env: env}), nil
}

// provideScriptEnv returns what the script helpers act through: the quest
// rates of server.properties, the party range and multiple item drop the
// kill rewards read, the item id allocator and the shared random source.
func provideScriptEnv(serverProps *config.Properties, rewards manager.KillRewardConfig, journals *script.Quests, ids *idfactory.Allocator) (*script.Env, error) {
	var rates script.Rates
	for _, r := range []struct {
		key string
		dst *float64
	}{
		{"RateQuestDrop", &rates.Drop},
		{"RateQuestReward", &rates.Reward},
		{"RateQuestRewardXP", &rates.XP},
		{"RateQuestRewardSP", &rates.SP},
		{"RateQuestRewardAdena", &rates.RewardAdena},
	} {
		v, err := serverProps.Float64(r.key, 1)
		if err != nil {
			return nil, err
		}
		*r.dst = v
	}
	return &script.Env{
		Quests:           journals,
		Rates:            rates,
		PartyRange:       rewards.PartyRange,
		MultipleItemDrop: rewards.MultipleItemDrop,
		NewItemID:        ids.NextID,
		Rand:             rnd.Get,
	}, nil
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

// provideMakers returns the maker registry: every npcmaker runs the maker
// of its type, the default maker when its type has none.
func provideMakers(log zerolog.Logger) *script.Makers {
	return script.NewMakers(maker.Catalog(), maker.Default, log)
}

// startSchedule runs the scheduled tasks against link. It starts once
// everything the tasks act on is restored (it is invoked after the Seven
// Signs, castles and clans start), so a task due at boot never stores
// state that is not loaded yet; on shutdown it stops before their final
// saves.
func startSchedule(lc fx.Lifecycle, scripts *script.Registry, link *network.GameClientLink, pool *sim.Pool) {
	queue := pool.NewQueue("script-schedule")
	var sch *script.Schedule
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			sch = script.StartSchedule(scripts, queue, link)
			return nil
		},
		OnStop: func(context.Context) error {
			sch.Stop()
			return nil
		},
	})
}

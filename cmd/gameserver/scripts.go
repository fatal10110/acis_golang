package main

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/guard"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monsterbehavior"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/catalog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/alliance"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/blackjudge"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/cabalbuffer"
	featureclan "github.com/fatal10110/acis_golang/internal/gameserver/script/feature/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/newbiehelper"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/tutorial"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q023"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q070"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q211"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q230"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q234"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q235"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q260"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q316"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q350"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q360"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q422"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q508"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q510"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q607"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q636"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q646"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q651"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
	scripttask "github.com/fatal10110/acis_golang/internal/gameserver/script/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/noblesseteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/primevalsummoner"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// scriptCatalogs are the literal script catalogs: the scripts ported with
// the engine, then the content catalog files; scriptCatalog joins them.
// crystals are the soul crystals a quest charges, buffs the buffs the
// newbie guides cast. The saga core and the sagas share one family.
func scriptCatalogs(crystals *item.SoulCrystalTable, buffs *skill.NewbieBuffTable) []script.Catalog {
	sagas := thirdclass.NewFamily()
	out := engineCatalogs(crystals, buffs, sagas)
	files := catalog.Scripts(sagas)
	for _, name := range slices.Sorted(maps.Keys(files)) {
		out = append(out, files[name])
	}
	return out
}

// engineCatalogs are the catalogs of the scripts ported with the engine.
func engineCatalogs(crystals *item.SoulCrystalTable, buffs *skill.NewbieBuffTable, sagas *thirdclass.Family) []script.Catalog {
	return []script.Catalog{questCatalog(crystals, sagas), featureCatalog(buffs), teleportCatalog(), taskCatalog(), aiCatalog()}
}

// questCatalog lists the quests ported with the engine.
func questCatalog(crystals *item.SoulCrystalTable, sagas *thirdclass.Family) script.Catalog {
	return script.Catalog{
		"quest.Q001_LettersOfLove":          q001.New,
		"quest.Q023_LidiasHeart":            q023.New,
		"quest.Q070_SagaOfThePhoenixKnight": func() script.Script { return q070.New(sagas) },
		"quest.Q211_TrialOfTheChallenger":   q211.New,
		"quest.Q230_TestOfTheSummoner":      q230.New,
		"quest.Q234_FatesWhisper":           q234.New,
		"quest.Q235_MimirsElixir":           q235.New,
		"quest.Q260_HuntTheOrcs":            q260.New,
		"quest.Q316_DestroyPlagueCarriers":  q316.New,
		"quest.Q350_EnhanceYourWeapon":      func() script.Script { return q350.New(crystals) },
		"quest.Q360_PlunderTheirSupplies":   q360.New,
		"quest.Q422_RepentYourSins":         q422.New,
		"quest.Q508_AClansReputation":       q508.New,
		"quest.Q510_AClansReputation":       q510.New,
		"quest.Q607_ProveYourCourage":       q607.New,
		"quest.Q636_TruthBeyondTheGate":     q636.New,
		"quest.Q646_SignsOfRevolt":          q646.New,
		"quest.Q651_RunawayYouth":           q651.New,
		"quest.ThirdClassQuest":             func() script.Script { return thirdclass.NewCore(sagas) },
	}
}

// featureCatalog lists the scripted features; the newbie guides cast
// buffs.
func featureCatalog(buffs *skill.NewbieBuffTable) script.Catalog {
	return script.Catalog{
		"script.feature.Alliance":     alliance.New,
		"script.feature.BlackJudge":   blackjudge.New,
		"script.feature.CabalBuffer":  cabalbuffer.New,
		"script.feature.Clan":         featureclan.New,
		"script.feature.NewbieHelper": func() script.Script { return newbiehelper.New(buffs) },
		"script.feature.Tutorial":     tutorial.New,
	}
}

// teleportCatalog lists the scripted teleporters.
func teleportCatalog() script.Catalog {
	return script.Catalog{
		"script.teleport.NoblesseTeleporter": noblesseteleporter.New,
		"script.teleport.PrimevalSummoner":   primevalsummoner.New,
	}
}

// aiCatalog lists the NPC behaviors.
func aiCatalog() script.Catalog {
	return script.Catalog{
		"script.ai.group.Walkers":                                                                        group.Walkers,
		"script.ai.individual.DefaultNpc":                                                                defaultnpc.New,
		"script.ai.individual.Guard.Guard":                                                               guard.Guard,
		"script.ai.individual.Guard.GuardMoveAround":                                                     guard.MoveAround,
		"script.ai.individual.Guard.GuardStand":                                                          guard.Stand,
		"script.ai.individual.Monster.MonsterAI":                                                         monster.New,
		"script.ai.individual.Monster.MonsterBehavior.MonsterBehavior":                                   monsterbehavior.MonsterBehavior,
		"script.ai.individual.Monster.MonsterBehavior.WarriorBehavior.WarriorBehavior":                   monsterbehavior.WarriorBehavior,
		"script.ai.individual.Monster.MonsterBehavior.WizardBehavior.WizardBehavior":                     monsterbehavior.WizardBehavior,
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateStandard":                                   raid.PrivateStandard,
		"script.ai.individual.Monster.WarriorBase.WarriorBase":                                           warrior.Base,
		"script.ai.individual.Monster.WarriorBase.Warrior.Warrior":                                       warrior.Warrior,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorAggressive":           warrior.Aggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorHold.WarriorHold":                       warrior.Hold,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecial": warrior.PhysicalSpecial,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorFishingBlock":    warrior.FishingBlock,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarrior":         warrior.PartyLeader,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateWarrior":       warrior.PartyPrivate,
		"script.ai.individual.Monster.WizardBase.WizardBase":                                             wizard.Base,
		"script.ai.individual.Monster.WizardBase.Wizard.Wizard":                                          wizard.Wizard,
	}
}

// taskCatalog lists the scheduled tasks. The shutdown and restart tasks
// run only once an operator uncomments their scripts.xml entries.
func taskCatalog() script.Catalog {
	return script.Catalog{
		"task.CastleTaxRefresh":     scripttask.CastleTaxRefresh,
		"task.ClanLadderRefresh":    scripttask.ClanLadderRefresh,
		"task.ClanLeaderTransfer":   scripttask.ClanLeaderTransfer,
		"task.RaidPointReset":       scripttask.RaidPointReset,
		"task.RecommendationUpdate": scripttask.RecommendationUpdate,
		"task.ServerRestart":        scripttask.ServerRestart,
		"task.ServerShutdown":       scripttask.ServerShutdown,
		"task.SevenSignsUpdate":     scripttask.SevenSignsUpdate,
	}
}

// scriptCatalog joins catalogs into one, without the entries not ported
// yet (a nil constructor). A path in two catalogs is an error, ported or
// not.
func scriptCatalog(catalogs []script.Catalog) (script.Catalog, error) {
	return joinCatalogs("script", catalogs)
}

// makerCatalog joins maker catalogs into one, as scriptCatalog joins
// script catalogs.
func makerCatalog(catalogs []script.MakerCatalog) (script.MakerCatalog, error) {
	return joinCatalogs("maker type", catalogs)
}

// joinCatalogs joins catalogs into one and drops the nil constructors; a
// key in two catalogs is an error.
func joinCatalogs[C ~map[string]func() T, T any](what string, catalogs []C) (C, error) {
	seen := map[string]bool{}
	out := C{}
	for _, c := range catalogs {
		for key, ctor := range c {
			if seen[key] {
				return nil, fmt.Errorf("%s %s is in two catalogs", what, key)
			}
			seen[key] = true
			if ctor != nil {
				out[key] = ctor
			}
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
//
// The engine's own work (the NPC dying hooks, and the script timers bound to
// no NPC or player queue) runs on an engine queue of pool, closed on
// shutdown before the persistence worker drains.
func provideScripts(lc fx.Lifecycle, paths gameServerPaths, data *gameData, env *script.Env, pool *sim.Pool, log zerolog.Logger) (*script.Registry, error) {
	list, err := gamexml.LoadScriptList(filepath.Join(paths.DataRoot, "data", "xml", "scripts.xml"), log)
	if err != nil {
		return nil, err
	}
	joined, err := scriptCatalog(scriptCatalogs(data.SoulCrystals, data.NewbieBuffs))
	if err != nil {
		return nil, err
	}
	engine := pool.NewQueue("script-engine")
	lc.Append(fx.Hook{OnStop: func(context.Context) error {
		engine.Close()
		return nil
	}})
	return script.Build(list, joined, script.Config{KindOf: npcKindOf(data.NPCs), Log: log, Env: env, HandleOps: handleOps(), Queue: engine}), nil
}

// handleOps returns the handle operations of every catalog script: those
// generated for scripts.go and those generated beside each content catalog
// file.
func handleOps() map[string]script.HandleOps {
	out := maps.Clone(scriptHandleOps)
	maps.Copy(out, catalog.HandleOps())
	return out
}

// provideScriptSpawner returns the spawner scripts place NPCs with. It is
// built ahead of the NPC population, which takes the script registry, and
// provideNpcs binds it to that population before the first NPC spawns.
func provideScriptSpawner(log zerolog.Logger) *script.Spawner {
	return script.NewSpawner(nil, log)
}

// provideScriptEnv returns what the script helpers act through: the quest
// rates of server.properties, the party range and multiple item drop the
// kill rewards read, the item id allocator, the NPC spawner, the shared
// random source, the world the scripts look NPCs up in, the skill and item
// definitions, the in-game clock and the wall clock, the datapack pages a
// script reads, the Seven Signs state, the grand boss wait times, the doors
// scripts open and close, the castles NPCs belong to with their sieges, and
// the clan halls with the siegable halls' sieges, and the clans.
func provideScriptEnv(serverProps *config.Properties, rewards manager.KillRewardConfig, journals *script.Quests, ids *idfactory.Allocator, spawner *script.Spawner, state *world.State, data *gameData, clock *task.GameClock, html *datacache.HTML, sevenSigns *sevensigns.State, gameplay gameplayConfig, doors *manager.WorldObjects, castles *castle.Manager, sieges *siege.Engine, halls *clanhall.Halls, clans *clan.Service) (*script.Env, error) {
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
		Quests:       journals,
		Rates:        rates,
		Drops:        rewards.Settings,
		NewItemID:    ids.NextID,
		Spawner:      spawner,
		Rand:         rnd.Get,
		World:        state,
		Skills:       data.Skills,
		GameHour:     clock.Hour,
		Uptime:       clock.Uptime,
		Now:          time.Now,
		Items:        data.Items,
		Page:         html.Get,
		SevenSigns:   sevenSigns,
		BossWaits:    gameplay.BossWaits,
		Doors:        doors,
		Castles:      castles,
		SiegableHall: data.ClanHalls.Siegable,
		Sieges:       sieges,
		Halls:        halls,
		Clans:        clanTable(clans),
		Zones:        data.Zones,
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
// of its type, the default maker when its type has none, reads the time of
// day from the game clock, finds the castles and their sieges and the
// doors, draws from the scripts' random source and reads the wall clock
// the NPC population runs on.
func provideMakers(clock *task.GameClock, castles *castle.Manager, sieges *siege.Engine, doors *manager.WorldObjects, log zerolog.Logger) (*script.Makers, error) {
	makers, err := makerCatalog(makerCatalogs())
	if err != nil {
		return nil, err
	}
	world := script.MakerWorld{Night: clock.IsNight, Castles: castles, Sieges: sieges, Doors: doors, Rand: rnd.Get, Now: time.Now}
	return script.NewMakers(makers, maker.Default, world, log), nil
}

// makerCatalogs are the maker catalogs: the makers ported with the engine,
// then the content catalog files.
func makerCatalogs() []script.MakerCatalog {
	out := []script.MakerCatalog{maker.Catalog()}
	files := catalog.Makers()
	for _, name := range slices.Sorted(maps.Keys(files)) {
		out = append(out, files[name])
	}
	return out
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

// clanTable is clans' table, none for no clans.
func clanTable(clans *clan.Service) script.Clans {
	if clans == nil {
		return nil
	}
	return clans.Table()
}

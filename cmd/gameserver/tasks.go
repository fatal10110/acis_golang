package main

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

func providePvPFlags(opts task.PvPFlagOptions) *task.PvPFlags {
	return task.NewPvPFlags(opts, time.Now)
}

func startPvPFlags(lc fx.Lifecycle, flags *task.PvPFlags, opts task.PvPFlagOptions, log zerolog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			for _, key := range opts.UnsupportedKeys {
				log.Warn().Str("file", "players.properties").Str("key", key).Msg("unsupported Karma/PvP config option")
			}
			return nil
		},
	})
	startTicker(lc, log, flags.Start)
}

// startTicker wires a component's fixed-interval task into the fx
// lifecycle: started once fx starts, stopped once fx stops.
func startTicker(lc fx.Lifecycle, log zerolog.Logger, start func(zerolog.Logger) *scheduler.Ticker) {
	var ticker *scheduler.Ticker
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ticker = start(log)
			return nil
		},
		OnStop: func(context.Context) error {
			if ticker != nil {
				ticker.Stop()
			}
			return nil
		},
	})
}

func provideWorldState() *world.State {
	return world.New()
}

// provideGroundItems restores dropped items persisted at the previous
// shutdown before the world starts, returning the store alongside so it can
// be reused to persist state back at the next shutdown.
func provideGroundItems(state *world.State, opts task.GroundItemOptions, pool *sql.DB, data *gameData, log zerolog.Logger) (*task.GroundItems, *gamesql.GroundItemStore, error) {
	store := gamesql.NewGroundItemStore(pool)
	items := task.NewGroundItems(state, opts, time.Now)

	rows, err := store.Load(context.Background())
	if err != nil {
		return nil, nil, err
	}
	if err := items.Load(rows, data.Items); err != nil {
		return nil, nil, err
	}
	if err := store.Clear(context.Background()); err != nil {
		return nil, nil, err
	}

	log.Info().Int("restored_ground_items", items.Len()).Msg("ground items restored")
	return items, store, nil
}

func startGroundItems(lc fx.Lifecycle, items *task.GroundItems, log zerolog.Logger) {
	startTicker(lc, log, items.Start)
}

// startGroundItemPersistence saves every currently tracked ground item back
// to items_on_ground at shutdown, mirroring the reference server's own
// save-on-shutdown behavior.
func startGroundItemPersistence(lc fx.Lifecycle, items *task.GroundItems, store *gamesql.GroundItemStore, log zerolog.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			if err := store.Save(ctx, items.Snapshots(nil)); err != nil {
				log.Warn().Err(err).Msg("save ground items")
			}
			return nil
		},
	})
}

func provideGameClock() *task.GameClock {
	return task.NewGameClock(time.Now)
}

func startGameClock(lc fx.Lifecycle, clock *task.GameClock, log zerolog.Logger) {
	startTicker(lc, log, func(log zerolog.Logger) *scheduler.Ticker {
		return scheduler.Start(task.GameMinute, clock.Tick, log)
	})
}

func providePlayerClock(clock *task.GameClock, state *world.State) (*task.PlayerClock, error) {
	return task.NewPlayerClock(clock, state, network.NewPlayerClockEffects(state))
}

// startPlayerClock forces PlayerClock construction before startGameClock
// launches the ticker; PlayerClock itself registers listeners on GameClock.
func startPlayerClock(_ *task.PlayerClock) {}

func provideWalker(data *gameData, state *world.State) (*task.Walker, error) {
	return task.NewWalker(data.Routes, task.GeoPath{Geo: data.Geo, Finder: data.Finder}, time.Now, state)
}

func startWalker(lc fx.Lifecycle, walker *task.Walker, log zerolog.Logger) {
	startTicker(lc, log, walker.Start)
}

type gameTaskEffects struct{}

func (gameTaskEffects) GaugeSet(task.WaterActor, time.Duration)  {}
func (gameTaskEffects) Drown(task.WaterActor)                    {}
func (gameTaskEffects) ManaThreshold(int32, *item.Instance, int) {}
func (gameTaskEffects) Expire(int32, *item.Instance)             {}

func provideWater() (*task.Water, error) {
	return task.NewWater(gameTaskEffects{}, time.Now)
}

func startWater(lc fx.Lifecycle, water *task.Water, log zerolog.Logger) {
	startTicker(lc, log, water.Start)
}

func provideShadowItems() (*task.ShadowItems, error) {
	return task.NewShadowItems(gameTaskEffects{})
}

func startShadowItems(lc fx.Lifecycle, items *task.ShadowItems, log zerolog.Logger) {
	startTicker(lc, log, items.Start)
}

// worldDecayEffects removes a decayed actor from the world once its corpse
// display interval elapses, and — once a spawn population has wired its own
// respawn resolution in via SetRespawnHook — arms that actor's next
// respawn. Actors without a decay hook are left alone: nothing outside the
// corpse-decay task itself is expected to register an actor that can't
// decay.
//
// The respawn hook is set after construction (manager.Npcs needs *task.Decay
// itself to register newly spawned actors, so it can't be built first), so
// it's guarded by its own lock separate from anything task.Decay holds.
type worldDecayEffects struct {
	state *world.State

	mu          sync.RWMutex
	respawnHook func(id int32) func()
}

type decayableActor interface {
	Decay(*world.State, func()) bool
}

func (w *worldDecayEffects) Decay(actor task.DecayActor) {
	obj, ok := w.state.Object(actor.ObjectID())
	if !ok {
		return
	}

	var respawn func()
	w.mu.RLock()
	hook := w.respawnHook
	w.mu.RUnlock()
	if hook != nil {
		respawn = hook(actor.ObjectID())
	}

	if d, ok := obj.(decayableActor); ok {
		d.Decay(w.state, respawn)
	}
}

// SetRespawnHook records the callback used to arm a decayed actor's next
// respawn. Call it once, before fx starts the decay ticker.
func (w *worldDecayEffects) SetRespawnHook(f func(id int32) func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.respawnHook = f
}

func provideDecay(state *world.State) (*task.Decay, *worldDecayEffects, error) {
	effects := &worldDecayEffects{state: state}
	d, err := task.NewDecay(effects, time.Now)
	return d, effects, err
}

func startDecay(lc fx.Lifecycle, d *task.Decay, log zerolog.Logger) {
	startTicker(lc, log, d.Start)
}

// npcRespawnEffects re-instantiates one spawn slot once its respawn
// deadline elapses. Like worldDecayEffects, its real resolution
// (manager.Npcs.Respawn) is wired in after construction, since Npcs itself
// needs *task.Respawn to schedule respawns in the first place.
type npcRespawnEffects struct {
	mu   sync.RWMutex
	hook func(key string)
}

func (e *npcRespawnEffects) Respawn(key string) {
	e.mu.RLock()
	hook := e.hook
	e.mu.RUnlock()
	if hook != nil {
		hook(key)
	}
}

// SetHook records the callback that re-instantiates a due spawn slot. Call
// it once, before fx starts the respawn ticker.
func (e *npcRespawnEffects) SetHook(f func(key string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.hook = f
}

func provideRespawnTask() (*task.Respawn, *npcRespawnEffects, error) {
	effects := &npcRespawnEffects{}
	r, err := task.NewRespawn(effects, time.Now)
	return r, effects, err
}

func startRespawnTask(lc fx.Lifecycle, r *task.Respawn, log zerolog.Logger) {
	startTicker(lc, log, r.Start)
}

func provideAI(state *world.State) *task.AI {
	return task.NewAI(state)
}

func startAI(lc fx.Lifecycle, ai *task.AI, log zerolog.Logger) {
	startTicker(lc, log, ai.Start)
}

func provideInventoryUpdates() *task.InventoryUpdates {
	return task.NewInventoryUpdates()
}

func startInventoryUpdates(lc fx.Lifecycle, updates *task.InventoryUpdates, log zerolog.Logger) {
	startTicker(lc, log, updates.Start)
}

// itemInstanceShutdownSaveTimeout bounds the final flush of pending item
// rows independently of however much of fx's stop timeout the earlier stop
// hooks have already spent.
const itemInstanceShutdownSaveTimeout = 10 * time.Second

// provideItemInstances builds the lazy item persistence task over the real
// items, augmentations and pets tables.
func provideItemInstances(pool *sql.DB, items *gamesql.ItemStore, data *gameData) *task.ItemInstances {
	return task.NewItemInstances(items, gamesql.NewAugmentationStore(pool), gamesql.NewPetStore(pool), data.Items)
}

// startItemInstances launches the persistence tick and flushes whatever is
// still pending at shutdown, matching the reference's shutdown sequence
// forcing one final ItemInstanceTaskManager save.
func startItemInstances(lc fx.Lifecycle, items *task.ItemInstances, log zerolog.Logger) {
	// Appended first so fx's reverse stop order runs it after the ticker
	// has stopped: the final save then sees a pending set nothing else is
	// still draining.
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			// This is the last chance to write these rows, and Save
			// releases the pending set either way, so a failure here is
			// lost data rather than a delay: it gets its own budget
			// (earlier stop hooks draining player containers can have
			// consumed most of fx's stop timeout by now) and is reported
			// rather than swallowed.
			saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), itemInstanceShutdownSaveTimeout)
			defer cancel()
			if err := items.Save(saveCtx); err != nil {
				log.Error().Err(err).Msg("save pending item instances")
				return err
			}
			return nil
		},
	})
	startTicker(lc, log, items.Start)
}

func providePositionUpdates(state *world.State) *task.PositionUpdates {
	return task.NewPositionUpdates(state)
}

func startPositionUpdates(lc fx.Lifecycle, positions *task.PositionUpdates, log zerolog.Logger) {
	startTicker(lc, log, positions.Start)
}

func provideEffects(state *world.State) *task.Effects {
	return task.NewEffects(state)
}

func startEffects(lc fx.Lifecycle, effects *task.Effects, log zerolog.Logger) {
	startTicker(lc, log, effects.Start)
}

// worldAttackStanceEffects stops an actor's attack animation once its
// combat-stance inactivity period elapses. Actors that don't expose a
// physical-attack controller are left alone.
type worldAttackStanceEffects struct{ state *world.State }

type attackStoppableActor interface {
	Stop()
}

func (w worldAttackStanceEffects) AutoAttackStop(actor task.AttackStanceActor) {
	obj, ok := w.state.Object(actor.ObjectID())
	if !ok {
		obj, ok = w.state.Player(actor.ObjectID())
		if !ok {
			return
		}
	}
	if s, ok := obj.(attackStoppableActor); ok {
		s.Stop()
	}
}

func provideAttackStance(state *world.State) (*task.AttackStance, error) {
	return task.NewAttackStance(worldAttackStanceEffects{state: state}, time.Now)
}

func startAttackStance(lc fx.Lifecycle, a *task.AttackStance, log zerolog.Logger) {
	startTicker(lc, log, a.Start)
}

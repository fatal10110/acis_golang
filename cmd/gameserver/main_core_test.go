package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// TestGameServerGraphValidates checks the fx constructor graph resolves
// without a database: every provider's dependencies are satisfied by some
// other provider, with no missing or duplicate types. go build only proves
// the Go code compiles, not that dig can wire it; this runs on every
// change to newGameServerAppOptions instead of only failing at boot.
func TestGameServerGraphValidates(t *testing.T) {
	if err := fx.ValidateApp(newGameServerAppOptions(gameServerPaths{})...); err != nil {
		t.Fatalf("fx graph does not resolve: %v", err)
	}
}

// TestGameServerStopTimeoutCoversEveryStopStep pins fx's stop budget against
// the stop hooks this package actually registers. fx checks its one stop
// deadline before each hook and skips the rest once it has expired, and Run
// then exits the process with any running hook cut off, so one hook that can
// wait longer than its share (a slow database under a save, a backed-up
// persistence lane) drops every save after it.
//
// The hooks are found by parsing this package's source for every OnStop
// field, keyed by the function that registers it, so a new or moved hook
// fails here until its bound is recorded. A hook with its own budget must
// reference that budget's constant; a hook with no bound of its own says why
// it needs none and falls under gameServerStopSlack.
func TestGameServerStopTimeoutCoversEveryStopStep(t *testing.T) {
	type stopBound struct {
		bound time.Duration
		uses  string // constant the registering function must reference; "" when the bound lives elsewhere
		why   string
	}
	hooks := map[string]stopBound{
		"startGameServer": {
			network.LivePlayerPersistWait, "",
			"waits for the connection handlers; each exit waits at most LivePlayerPersistWait for its player's saves, in parallel, and cancelling closes the login link so its writes fail fast",
		},
		"startDebugHTTP":           {debugHTTPStopTimeout, "debugHTTPStopTimeout", "graceful stop of the debug listener"},
		"startNpcPersistence":      {shutdownSaveTimeout, "shutdownSaveTimeout", "spawn_data save"},
		"startRelationPersistence": {shutdownSaveTimeout, "shutdownSaveTimeout", "character_relations save"},
		"startSimPool":             {simPoolStopTimeout, "simPoolStopTimeout", "actor pool finishing queued tasks"},
		"startTicker": {
			task.ItemInstanceSaveTimeout, "",
			"StopAndWait waits for one in-flight tick; the item tick is the only one with database I/O, bounded by ItemInstanceSaveTimeout, and the rest are in-memory",
		},
		"startItemInstances":         {3 * task.ItemInstanceSaveTimeout, "ItemInstanceSaveTimeout", "drainItemInstances: save, persistence-worker drain, save"},
		"providePersist":             {persistCloseTimeout, "persistCloseTimeout", "persistence worker's last close"},
		"startGroundItemPersistence": {shutdownSaveTimeout, "shutdownSaveTimeout", "items_on_ground save"},
		"startSevenSigns":            {0, "", "stops a timer under a lock the status save does not hold across its write"},
		"provideGameServerLogger":    {0, "", "closes the log file"},
		"provideBootContext":         {0, "", "cancels a context"},
		"provideGameServerDatabase":  {0, "", "closes the pool; the last database step, so running past the deadline loses nothing"},
	}

	registered := stopHookRegistrars(t)
	sum := gameServerStopSlack
	for name, b := range hooks {
		refs, ok := registered[name]
		if !ok {
			t.Errorf("%s is listed but registers no OnStop hook", name)
			continue
		}
		if b.uses != "" && !refs[b.uses] {
			t.Errorf("%s's stop hook does not reference %s, the bound it is budgeted for (%s)", name, b.uses, b.why)
		}
		sum += b.bound
	}
	for name := range registered {
		if _, ok := hooks[name]; !ok {
			t.Errorf("%s registers an OnStop hook with no recorded bound: give it its own budget and add it to gameServerStopTimeout", name)
		}
	}
	if gameServerStopTimeout < sum {
		t.Fatalf("gameServerStopTimeout = %s, below the %s its stop hooks can take", gameServerStopTimeout, sum)
	}
}

// stopHookRegistrars parses this package's non-test source and returns, for
// every function containing an OnStop field, the identifiers it references.
func stopHookRegistrars(t *testing.T) map[string]map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	out := make(map[string]map[string]bool)
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			refs := make(map[string]bool)
			hasStop := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.KeyValueExpr:
					if key, ok := n.Key.(*ast.Ident); ok && key.Name == "OnStop" {
						hasStop = true
					}
				case *ast.Ident:
					refs[n.Name] = true
				}
				return true
			})
			if hasStop {
				out[fn.Name.Name] = refs
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("found no OnStop hooks; is the test running in cmd/gameserver?")
	}
	return out
}

// TestSlowSpawnSaveLeavesItemDrainItsStopBudget stops an fx app whose hooks
// sit in the game server's order: the spawn-data save first, then the final
// item drain. The spawn save's database never answers and an owner's
// persistence lane is held past the drain's first save. The stop budget is
// the sum of the steps' own bounds, so fx still reaches the drain and the
// pending item row is written. A spawn save on fx's own stop context would
// spend the whole budget, and fx would then skip the drain.
func TestSlowSpawnSaveLeavesItemDrainItsStopBudget(t *testing.T) {
	const budget = 200 * time.Millisecond
	worker := persist.New(zerolog.Nop())
	flusher := &countingItemFlusher{}
	items := task.NewItemInstances(flusher, item.NewTable(nil), worker, nil, zerolog.Nop())
	inst := &item.Instance{ObjectID: 1, TemplateID: 1, OwnerID: 7, Count: 1, Location: item.LocationInventory}
	items.Add(inst)
	// Held through the spawn save and the drain's first save.
	worker.Enqueue(inst.OwnerID, func() { time.Sleep(2*budget + budget/2) })

	app := fx.New(fx.NopLogger, fx.Invoke(func(lc fx.Lifecycle) {
		lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
			return drainItemInstances(ctx, items, worker, zerolog.Nop(), budget)
		}})
		lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
			saveOnStop(ctx, budget, zerolog.Nop(), "save spawn data", func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			})
			return nil
		}})
	}))
	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), budget+3*budget+budget)
	defer cancel()
	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("stop error = %v, want every hook run inside the stop budget", err)
	}
	if got := flusher.count(); got != 1 {
		t.Fatalf("item flushes = %d, want 1 from the drain", got)
	}
	if items.Contains(inst) {
		t.Fatal("item still pending after shutdown")
	}
}

// TestSlowPersistCloseLeavesGroundSaveItsStopBudget stops an fx app whose
// hooks sit in the game server's order: the item drain, then the
// persistence worker's last close, then the ground-item save. One lane is
// backed up far past every budget, so both closes give up. Each close is
// bounded, so fx still reaches the ground-item save inside the stop budget.
// A close on fx's own stop context would wait out the whole budget, and fx
// would skip the save; items_on_ground was cleared at boot, so every ground
// item would be lost.
func TestSlowPersistCloseLeavesGroundSaveItsStopBudget(t *testing.T) {
	const budget = 100 * time.Millisecond
	worker := persist.New(zerolog.Nop())
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	worker.Enqueue(7, func() { <-release })
	items := task.NewItemInstances(&countingItemFlusher{}, item.NewTable(nil), worker, nil, zerolog.Nop())

	var groundSaved atomic.Bool
	app := fx.New(fx.NopLogger, fx.Invoke(func(lc fx.Lifecycle) {
		lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
			saveOnStop(ctx, budget, zerolog.Nop(), "save ground items", func(context.Context) error {
				groundSaved.Store(true)
				return nil
			})
			return nil
		}})
		lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
			return closePersistOnStop(ctx, worker, budget)
		}})
		lc.Append(fx.Hook{OnStop: func(ctx context.Context) error {
			return drainItemInstances(ctx, items, worker, zerolog.Nop(), budget)
		}})
	}))
	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 3*budget+budget+budget+2*budget)
	defer cancel()
	_ = app.Stop(stopCtx) // both closes report the backed-up lane
	if stopCtx.Err() != nil {
		t.Fatal("stop budget ran out before the last hook")
	}
	if !groundSaved.Load() {
		t.Fatal("ground-item save never ran")
	}
}

// TestDrainItemInstancesOutlivesExpiredStopContext runs the shutdown item
// drain with fx's stop ctx already expired and an owner's persistence lane
// held longer than one step's budget. The first save gives up behind the
// lane, and its owner job returns the item to pending when it runs. The drain
// must then wait for the worker on its own budget and write the item in the
// retry save: draining with the expired stop ctx returns at once and leaves
// the item unwritten.
func TestDrainItemInstancesOutlivesExpiredStopContext(t *testing.T) {
	worker := persist.New(zerolog.Nop())
	flusher := &countingItemFlusher{}
	items := task.NewItemInstances(flusher, item.NewTable(nil), worker, nil, zerolog.Nop())
	inst := &item.Instance{ObjectID: 1, TemplateID: 1, OwnerID: 7, Count: 1, Location: item.LocationInventory}
	items.Add(inst)

	const budget = 200 * time.Millisecond
	worker.Enqueue(inst.OwnerID, func() { time.Sleep(budget + budget/2) })
	stopCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := drainItemInstances(stopCtx, items, worker, zerolog.Nop(), budget); err != nil {
		t.Fatalf("drain error = %v", err)
	}
	if got := flusher.count(); got != 1 {
		t.Fatalf("item flushes = %d, want 1 from the retry save after the drain", got)
	}
	if items.Contains(inst) {
		t.Fatal("item still pending after the drain")
	}
}

type countingItemFlusher struct {
	mu sync.Mutex
	n  int
}

func (f *countingItemFlusher) Flush(context.Context, item.FlushBatch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	return nil
}

func (f *countingItemFlusher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

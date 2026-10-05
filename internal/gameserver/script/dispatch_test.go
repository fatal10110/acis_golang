package script

import (
	"context"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		answer string
		want   Result
	}{
		{"", Result{Kind: ResultNone}},
		{"30006-01.htm", Result{ResultPageFile, "30006-01.htm"}},
		{"30006-01.html", Result{ResultPageFile, "30006-01.html"}},
		{"<html><body>hi</body></html>", Result{ResultPage, "<html><body>hi</body></html>"}},
		// The file test comes first: a page ending in .htm is a file name.
		{"<html>x.htm", Result{ResultPageFile, "<html>x.htm"}},
		{"You are too weak.", Result{ResultChat, "You are too weak."}},
		{" <html>", Result{ResultChat, " <html>"}},
		{"page.HTM", Result{ResultChat, "page.HTM"}},
	} {
		if got := Classify(tc.answer); got != tc.want {
			t.Errorf("Classify(%q) = %+v, want %+v", tc.answer, got, tc.want)
		}
	}
}

// TestPanickingHookIsIsolated: a panic is recovered and logged with its
// stack, the next hook in the list still runs, the script keeps running on
// later invocations, and a string hook that panics answers ResultAborted.
func TestPanickingHookIsIsolated(t *testing.T) {
	var ran []string
	var calls atomic.Int32
	catalog := Catalog{
		"ai.Panics": func() Script {
			return Script{Behavior: true, NPCs: []int32{1}, Hooks: Hooks{OnSeeSpell: func(*Script, SeeSpell) {}, OnAbnormalStatusChanged: func(*Script, AbnormalStatusChanged) {
				calls.Add(1)
				panic("boom")
			}}}
		},
		"quest.After": func() Script {
			return Script{Bind: Bindings{EventSeeSpell: {1}}, Hooks: Hooks{OnAbnormalStatusChanged: func(*Script, AbnormalStatusChanged) { ran = append(ran, "quest.After") }}}
		},
		"script.Talker": func() Script {
			return Script{Bind: Bindings{EventFirstTalk: {2}}, Hooks: Hooks{OnFirstTalk: func(*Script, FirstTalk) string {
				var p *Player
				_ = *p // a nil dereference, as a script bug would make
				return "never.htm"
			}}}
		},
	}
	r, logs := build(t, listOf("ai.Panics", "quest.After", "script.Talker"), catalog)

	r.AbnormalStatusChanged(1, AbnormalStatusChanged{})
	r.AbnormalStatusChanged(1, AbnormalStatusChanged{})
	if calls.Load() != 2 || !slices.Equal(ran, []string{"quest.After", "quest.After"}) {
		t.Fatalf("panicking hook ran %d times and the next hook ran %v; want 2 and twice", calls.Load(), ran)
	}
	out := logs.String()
	if n := strings.Count(out, `"script":"ai.Panics","hook":"onAbnormalStatusChanged","panic":"boom"`); n != 2 {
		t.Fatalf("panic logged %d times, want 2: %s", n, out)
	}
	if !strings.Contains(out, `"stack":"goroutine `) || !strings.Contains(out, "dispatch_test.go") {
		t.Fatalf("panic logged without the hook's stack: %s", out)
	}

	res, bound := r.FirstTalk(2, FirstTalk{})
	if !bound || res != (Result{Kind: ResultAborted}) {
		t.Fatalf("panicking first talk = %+v, %v; want ResultAborted", res, bound)
	}
}

// TestDispatchFromManyQueues dispatches from several queues at once on both
// executors against one registry, with a panicking hook in every list. Run
// it with -race.
func TestDispatchFromManyQueues(t *testing.T) {
	const queues, posts = 8, 200
	newRegistry := func(t *testing.T) (*Registry, *atomic.Int64, *atomic.Int64) {
		var reached, talked atomic.Int64
		catalog := Catalog{
			"ai.Panics": func() Script {
				return Script{Behavior: true, NPCs: []int32{1, 2}, Hooks: Hooks{
					OnSeeSpell:              func(*Script, SeeSpell) {},
					OnAbnormalStatusChanged: func(*Script, AbnormalStatusChanged) { panic("boom") },
					OnFirstTalk:             func(*Script, FirstTalk) string { panic("boom") },
				}}
			},
			"quest.Counts": func() Script {
				return Script{Bind: Bindings{EventSeeSpell: {1, 2}, EventFirstTalk: {3}}, Hooks: Hooks{
					OnAbnormalStatusChanged: func(*Script, AbnormalStatusChanged) { reached.Add(1) },
					OnFirstTalk:             func(*Script, FirstTalk) string { talked.Add(1); return "ok.htm" },
				}}
			},
		}
		r := Build(listOf("ai.Panics", "quest.Counts"), catalog, Config{KindOf: allTemplates, Log: zerolog.Nop(), raises: raiseAll})
		return r, &reached, &talked
	}
	dispatch := func(t *testing.T, r *Registry, npc int32) {
		r.AbnormalStatusChanged(npc, AbnormalStatusChanged{})
		if res, bound := r.FirstTalk(3, FirstTalk{}); !bound || res.Kind != ResultPageFile {
			t.Errorf("first talk = %+v, %v", res, bound)
		}
		if res, bound := r.FirstTalk(1, FirstTalk{}); !bound || res.Kind != ResultAborted {
			t.Errorf("panicking first talk = %+v, %v", res, bound)
		}
	}
	check := func(t *testing.T, reached, talked *atomic.Int64) {
		if got := reached.Load(); got != queues*posts {
			t.Errorf("abnormal status reached the counting script %d times, want %d", got, queues*posts)
		}
		if got := talked.Load(); got != queues*posts {
			t.Errorf("first talk ran %d times, want %d", got, queues*posts)
		}
	}

	t.Run("inline", func(t *testing.T) {
		r, reached, talked := newRegistry(t)
		loop := sim.NewInline(time.Unix(0, 0))
		var wg sync.WaitGroup
		for q := range queues {
			queue := loop.NewQueue("npc")
			wg.Go(func() {
				for range posts {
					queue.Post(func() { dispatch(t, r, int32(q%2+1)) })
				}
			})
		}
		wg.Wait()
		loop.Run()
		check(t, reached, talked)
	})

	t.Run("pool", func(t *testing.T) {
		r, reached, talked := newRegistry(t)
		pool := sim.NewPool(4, zerolog.Nop())
		pool.Start(context.Background())
		var done sync.WaitGroup
		done.Add(queues * posts)
		for q := range queues {
			queue := pool.NewQueue("npc")
			go func() {
				for range posts {
					queue.Post(func() {
						defer done.Done()
						dispatch(t, r, int32(q%2+1))
					})
				}
			}()
		}
		done.Wait()
		if err := pool.Stop(context.Background()); err != nil {
			t.Fatal(err)
		}
		check(t, reached, talked)
	})
}

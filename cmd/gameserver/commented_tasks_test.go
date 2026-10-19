package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// commentedTasks are the reference tasks the shipped scripts.xml lists only
// in a comment. An operator who uncomments the entry gets the task, so the
// catalog holds them although nothing listed builds them; the catalog
// checks exempt exactly these.
var commentedTasks = []string{"task.ServerRestart", "task.ServerShutdown"}

// commentedEntry matches the commented-out scripts.xml entry of path, the
// entry itself in group 1.
func commentedEntry(path string) *regexp.Regexp {
	return regexp.MustCompile(`<!--\s*(<script path="` + regexp.QuoteMeta(path) + `"[^>]*/>)\s*-->`)
}

// TestCommentedTasksAreCommentedEntries keeps the exemption honest: each
// exempt task has exactly one commented-out entry in the shipped
// scripts.xml, and no live entry.
func TestCommentedTasksAreCommentedEntries(t *testing.T) {
	file := datapack.Path(t, "data", "xml", "scripts.xml")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	list, err := gamexml.LoadScriptList(file, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range commentedTasks {
		if n := len(commentedEntry(path).FindAll(raw, -1)); n != 1 {
			t.Errorf("scripts.xml has %d commented-out entries of %s, want 1", n, path)
		}
		if slices.ContainsFunc(list, func(l script.Listing) bool { return l.Path == path }) {
			t.Errorf("scripts.xml lists %s; drop it from commentedTasks", path)
		}
	}
}

// shutdownRecorder is the server a task acts on, recording its countdown
// starts.
type shutdownRecorder struct {
	script.Server
	calls []string
}

func (r *shutdownRecorder) Shutdown(seconds int32, restart bool) {
	r.calls = append(r.calls, fmt.Sprintf("Shutdown %d %v", seconds, restart))
}

// TestCommentedTasksRunOnceUncommented: with its shipped entry uncommented,
// each commented task is built from the boot's catalog, scheduled, and at
// its start (Tuesday 17:00) starts the ten-minute countdown, restarting for
// the restart task. The countdown itself is pinned in tests/admin.
func TestCommentedTasksRunOnceUncommented(t *testing.T) {
	raw, err := os.ReadFile(datapack.Path(t, "data", "xml", "scripts.xml"))
	if err != nil {
		t.Fatal(err)
	}
	joined, err := scriptCatalog(scriptCatalogs(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		want string
	}{
		{"task.ServerRestart", "Shutdown 600 true"},
		{"task.ServerShutdown", "Shutdown 600 false"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "scripts.xml")
			if err := os.WriteFile(file, commentedEntry(tc.path).ReplaceAll(raw, []byte("$1")), 0o644); err != nil {
				t.Fatal(err)
			}
			list, err := gamexml.LoadScriptList(file, zerolog.Nop())
			if err != nil {
				t.Fatal(err)
			}
			i := slices.IndexFunc(list, func(l script.Listing) bool { return l.Path == tc.path })
			if i < 0 {
				t.Fatalf("the uncommented scripts.xml does not list %s", tc.path)
			}
			noTemplates := func(int32) (script.NPCKind, bool) { return script.KindOther, false }
			r := script.Build(list[i:i+1], joined, script.Config{KindOf: noTemplates, Log: zerolog.Nop(), HandleOps: handleOps()})

			// Tuesday 2026-10-20, a minute before the entry's start.
			in := sim.NewInline(time.Date(2026, 10, 20, 16, 59, 0, 0, time.UTC))
			srv := &shutdownRecorder{}
			sch := script.StartSchedule(r, in.NewQueue("schedule"), srv)
			defer sch.Stop()
			in.Advance(30 * time.Second)
			if len(srv.calls) != 0 {
				t.Fatalf("before its start the task ran %q", srv.calls)
			}
			in.Advance(time.Minute)
			if !slices.Equal(srv.calls, []string{tc.want}) {
				t.Fatalf("at its start the task ran %q, want %q", srv.calls, tc.want)
			}
		})
	}
}

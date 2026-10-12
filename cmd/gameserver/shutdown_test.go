package main

import (
	"testing"

	"github.com/rs/zerolog"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

// TestStopProcessExitCodes pins the exit status an admin stop asks the app
// for: 0 for a shutdown, restartExitCode for a restart, so a supervisor
// restarting on that status restarts only the latter.
func TestStopProcessExitCodes(t *testing.T) {
	for _, tc := range []struct {
		restart bool
		want    int
	}{{false, 0}, {true, restartExitCode}} {
		var shutdowner fx.Shutdowner
		app := fxtest.New(t, fx.Populate(&shutdowner))
		app.RequireStart()
		stopProcess(shutdowner, zerolog.Nop())(tc.restart)
		if got := (<-app.Wait()).ExitCode; got != tc.want {
			t.Errorf("stop with restart=%v exits %d, want %d", tc.restart, got, tc.want)
		}
		app.RequireStop()
	}
}

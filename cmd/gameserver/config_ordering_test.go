package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// TestLoadPvPFlagOptionsAndPetConfigReadAfterLoggerIsConfigured guards the
// review finding on PR #2293 (fatal10110/acis_golang): loadPvPFlagOptions and
// loadPetConfig originally took only gameServerPaths, so nothing in the fx
// graph ordered them after provideGameServerLogger installs the process
// logger via config.SetLogger. Any warnMissing they triggered went to
// config's unconfigured default logger (raw os.Stderr) instead of the
// caller-supplied sink, the same bypass issue #2283 asked to remove.
//
// This wires a minimal fx app shaped like the real one — a logger provider
// that calls config.SetLogger, plus the two config loaders — and asserts
// their missing-key warnings land in the configured sink. Before the fix
// (loaders without the ordering parameter) this failed because dig could
// resolve either loader before the logger provider.
func TestLoadPvPFlagOptionsAndPetConfigReadAfterLoggerIsConfigured(t *testing.T) {
	previous := zerolog.Nop()
	t.Cleanup(func() { config.SetLogger(previous) })

	dir := t.TempDir()
	// Empty properties files: every key loadPvPFlagOptions/loadPetConfig
	// read is missing, so each one raises a warnMissing.
	playersPath := filepath.Join(dir, "players.properties")
	if err := os.WriteFile(playersPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	serverPath := filepath.Join(dir, "server.properties")
	if err := os.WriteFile(serverPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	paths := gameServerPaths{PlayersConfigPath: playersPath, ConfigPath: serverPath}

	var configured bytes.Buffer
	app := fx.New(
		fx.NopLogger,
		fx.Supply(paths),
		fx.Provide(
			func() zerolog.Logger {
				l := zerolog.New(&configured)
				config.SetLogger(l)
				return l
			},
			loadPvPFlagOptions,
			loadPetConfig,
		),
		fx.Invoke(func(task.PvPFlagOptions, pet.Config) {}),
	)

	ctx := context.Background()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("app.Start: %v", err)
	}
	t.Cleanup(func() { _ = app.Stop(ctx) })

	if !strings.Contains(configured.String(), "config property missing") {
		t.Fatalf("missing-key warnings did not reach the configured logger; got %q", configured.String())
	}
}

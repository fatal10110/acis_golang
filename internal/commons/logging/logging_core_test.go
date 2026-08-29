package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/rs/zerolog"
)

// ---- from logging_test.go ----
func TestConfigFromProperties(t *testing.T) {
	props, err := config.ParseString(`
.level = INFO
java.util.logging.FileHandler.pattern = log/console/console_%g.txt
net.sf.l2j.commons.logging.handler.ErrorLogHandler.pattern = log/error/error_%g.txt
net.sf.l2j.commons.logging.handler.ChatLogHandler.pattern = log/chat/chat_%g.txt
net.sf.l2j.commons.logging.handler.GMAuditLogHandler.pattern = log/gmaudit/gmaudit_%g.txt
net.sf.l2j.commons.logging.handler.ItemLogHandler.pattern = log/item/item_%g.txt
unknown = value
`)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := ConfigFromProperties(props)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != zerolog.InfoLevel {
		t.Fatalf("Level = %s, want info", cfg.Level)
	}
	if got := cfg.Patterns[SinkConsole]; got != "log/console/console_%g.txt" {
		t.Fatalf("console pattern = %q", got)
	}
	if !reflect.DeepEqual(cfg.UnsupportedKeys, []string{"unknown"}) {
		t.Fatalf("UnsupportedKeys = %#v", cfg.UnsupportedKeys)
	}
}

func TestSetupRoutesStructuredLogs(t *testing.T) {
	props, err := config.ParseString(`
.level = CONFIG
java.util.logging.FileHandler.pattern = log/console/console_%g.txt
net.sf.l2j.commons.logging.handler.ErrorLogHandler.pattern = log/error/error_%g.txt
net.sf.l2j.commons.logging.handler.ChatLogHandler.pattern = log/chat/chat_%g.txt
net.sf.l2j.commons.logging.handler.GMAuditLogHandler.pattern = log/gmaudit/gmaudit_%g.txt
net.sf.l2j.commons.logging.handler.ItemLogHandler.pattern = log/item/item_%g.txt
`)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ConfigFromProperties(props)
	if err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	runtime, err := Setup(t.TempDir(), cfg, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	runtime.Logger.Info().Int("port", 7777).Msg("server started")
	runtime.Logger.Error().Msg("boot failed")
	runtime.Logger.Error().Err(errors.New("disk full")).Msg("write failed")
	runtime.Chat.Info().Str("from", "player").Msg("hello")
	runtime.GMAudit.Info().Str("gm", "admin").Msg("teleport")
	runtime.Item.Info().Int("item", 57).Msg("add")

	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}

	assertFileContains(t, runtime.Path(SinkConsole), "server started")
	assertFileNotContains(t, runtime.Path(SinkError), "boot failed")
	assertFileContains(t, runtime.Path(SinkError), "write failed")
	assertFileContains(t, runtime.Path(SinkChat), "hello")
	assertFileContains(t, runtime.Path(SinkGMAudit), "teleport")
	assertFileContains(t, runtime.Path(SinkItem), "add")
	if !strings.Contains(stderr.String(), "server started") {
		t.Fatalf("stderr = %q, want server log", stderr.String())
	}
}

func TestSetupRoutesWarningWithErrorToErrorFile(t *testing.T) {
	props, err := config.ParseString(`
.level = CONFIG
net.sf.l2j.commons.logging.handler.ErrorLogHandler.level = CONFIG
`)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ConfigFromProperties(props)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := Setup(t.TempDir(), cfg, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	runtime.Logger.Warn().Err(errors.New("disk full")).Msg("write delayed")
	assertFileContains(t, runtime.Path(SinkError), "write delayed")
}

func TestSetupAppliesDedicatedHandlerLevels(t *testing.T) {
	props, err := config.ParseString(`
.level = CONFIG
java.util.logging.ConsoleHandler.level = INFO
java.util.logging.FileHandler.level = INFO
net.sf.l2j.commons.logging.handler.ChatLogHandler.level = INFO
net.sf.l2j.commons.logging.handler.GMAuditLogHandler.level = INFO
net.sf.l2j.commons.logging.handler.ItemLogHandler.level = INFO
`)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ConfigFromProperties(props)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	runtime, err := Setup(t.TempDir(), cfg, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	runtime.Logger.Debug().Msg("hidden main")
	runtime.Chat.Debug().Msg("hidden chat")
	runtime.GMAudit.Debug().Msg("hidden audit")
	runtime.Item.Debug().Msg("hidden item")
	assertFileNotContains(t, runtime.Path(SinkConsole), "hidden main")
	if strings.Contains(stderr.String(), "hidden main") {
		t.Fatalf("stderr = %q, did not want debug record", stderr.String())
	}
	assertFileNotContains(t, runtime.Path(SinkChat), "hidden chat")
	assertFileNotContains(t, runtime.Path(SinkGMAudit), "hidden audit")
	assertFileNotContains(t, runtime.Path(SinkItem), "hidden item")
}

func TestSetupAppliesErrorHandlerLevel(t *testing.T) {
	props, err := config.ParseString(`
.level = CONFIG
net.sf.l2j.commons.logging.handler.ErrorLogHandler.level = OFF
`)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ConfigFromProperties(props)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := Setup(t.TempDir(), cfg, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	runtime.Logger.Error().Err(errors.New("disk full")).Msg("write failed")
	assertFileNotContains(t, runtime.Path(SinkError), "write failed")
}

func TestConfigParsesRotationKeys(t *testing.T) {
	props, err := config.ParseString(`
java.util.logging.FileHandler.limit = 1000000
java.util.logging.FileHandler.count = 5
net.sf.l2j.commons.logging.handler.ErrorLogHandler.limit = 1000000
net.sf.l2j.commons.logging.handler.ErrorLogHandler.count = 5
net.sf.l2j.commons.logging.handler.ChatLogHandler.limit = 1000000
net.sf.l2j.commons.logging.handler.ChatLogHandler.count = 5
net.sf.l2j.commons.logging.handler.ChatLogHandler.append = true
net.sf.l2j.commons.logging.handler.GMAuditLogHandler.limit = 1000000
net.sf.l2j.commons.logging.handler.GMAuditLogHandler.count = 5
net.sf.l2j.commons.logging.handler.GMAuditLogHandler.append = true
net.sf.l2j.commons.logging.handler.ItemLogHandler.limit = 1000000
net.sf.l2j.commons.logging.handler.ItemLogHandler.count = 5
net.sf.l2j.commons.logging.handler.ItemLogHandler.append = true
`)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ConfigFromProperties(props)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.UnsupportedKeys) != 0 {
		t.Fatalf("UnsupportedKeys = %#v, want none", cfg.UnsupportedKeys)
	}
	for _, sink := range []Sink{SinkConsole, SinkError, SinkChat, SinkGMAudit, SinkItem} {
		if cfg.Limits[sink] != 1000000 {
			t.Fatalf("Limits[%s] = %d, want 1000000", sink, cfg.Limits[sink])
		}
		if cfg.Counts[sink] != 5 {
			t.Fatalf("Counts[%s] = %d, want 5", sink, cfg.Counts[sink])
		}
	}
	if cfg.Append[SinkConsole] || cfg.Append[SinkError] {
		t.Fatalf("console/error append should default to false: %#v", cfg.Append)
	}
	if !cfg.Append[SinkChat] || !cfg.Append[SinkGMAudit] || !cfg.Append[SinkItem] {
		t.Fatalf("chat/gmaudit/item append should be true: %#v", cfg.Append)
	}
}

func TestSetupRotatesFileWhenLimitExceeded(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Limits[SinkConsole] = 10
	cfg.Counts[SinkConsole] = 3

	runtime, err := Setup(dir, cfg, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	for range 5 {
		runtime.Logger.Info().Msg("0123456789")
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}

	base := filepath.Join(dir, "log", "console")
	for _, gen := range []string{"console_0.txt", "console_1.txt", "console_2.txt"} {
		if _, err := os.Stat(filepath.Join(base, gen)); err != nil {
			t.Fatalf("expected rotated file %s: %v", gen, err)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "console_3.txt")); !os.IsNotExist(err) {
		t.Fatalf("generation beyond count should not exist, err=%v", err)
	}
}

func TestSetupAppendPreservesExistingGenerationZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log", "chat")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "chat_0.txt"), []byte("prior run\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.Append[SinkChat] = true

	runtime, err := Setup(dir, cfg, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runtime.Chat.Info().Msg("new line")
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}

	assertFileContains(t, runtime.Path(SinkChat), "prior run")
	assertFileContains(t, runtime.Path(SinkChat), "new line")
}

func TestSetupNoAppendTruncatesExistingGenerationZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log", "error")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "error_0.txt"), []byte("stale content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	// SinkError append defaults to false, matching FileHandler's class default.

	runtime, err := Setup(dir, cfg, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runtime.Logger.Error().Msg("fresh content")
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(runtime.Path(SinkError))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "stale content") {
		t.Fatalf("error file = %q, want stale content truncated", data)
	}
}

func TestSetupSuppressesDebugBelowInfo(t *testing.T) {
	runtime, err := Setup(t.TempDir(), DefaultConfig(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	runtime.Logger.Debug().Msg("hidden")
	data, err := os.ReadFile(runtime.Path(SinkConsole))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "hidden") {
		t.Fatalf("console = %q, debug record was written", data)
	}
}

func TestSetupPreservesJSONLogFormat(t *testing.T) {
	runtime, err := Setup(t.TempDir(), DefaultConfig(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	runtime.Logger.Info().Msg("server started")
	data, err := os.ReadFile(runtime.Path(SinkConsole))
	if err != nil {
		t.Fatal(err)
	}

	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	if got := event["msg"]; got != "server started" {
		t.Fatalf("msg = %#v, want %q", got, "server started")
	}
	if _, ok := event["message"]; ok {
		t.Fatalf("event has unexpected message key: %s", data)
	}
	timestamp, ok := event["time"].(string)
	if !ok {
		t.Fatalf("time = %#v, want RFC3339 string", event["time"])
	}
	if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
		t.Fatalf("time = %q, want RFC3339: %v", timestamp, err)
	}
}

func TestBadLevelFails(t *testing.T) {
	props, err := config.ParseString(".level = LOUD\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConfigFromProperties(props); err == nil {
		t.Fatal("ConfigFromProperties with bad level: expected error")
	}
}

func TestLoggingPropertiesFromEnvironment(t *testing.T) {
	name := os.Getenv("ACIS_LOGGING_PROPERTIES")
	if name == "" {
		t.Skip("set ACIS_LOGGING_PROPERTIES to smoke-test real logging.properties")
	}

	props, err := config.LoadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ConfigFromProperties(props)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.UnsupportedKeys) != 0 {
		t.Fatalf("UnsupportedKeys = %#v", cfg.UnsupportedKeys)
	}

	runtime, err := Setup(t.TempDir(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runtime.Logger.Info().Msg("smoke")
	runtime.Chat.Info().Msg("smoke chat")
	runtime.GMAudit.Info().Msg("smoke audit")
	runtime.Item.Info().Msg("smoke item")
}

func BenchmarkLoggerLevels(b *testing.B) {
	logger := zerolog.New(io.Discard).Level(zerolog.InfoLevel)
	b.Run("info", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Info().Msg("accepted connection")
		}
	})
	b.Run("error", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Error().Msg("connection failed")
		}
	})
	b.Run("debug suppressed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Debug().Msg("packet received")
		}
	})
}

func assertFileContains(t *testing.T, name, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(name))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s = %q, want %q", name, string(data), want)
	}
}

func assertFileNotContains(t *testing.T, name, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(name))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), want) {
		t.Fatalf("%s = %q, did not want %q", name, string(data), want)
	}
}

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

var unsupportedRotationKeys = []string{
	"java.util.logging.FileHandler.count",
	"java.util.logging.FileHandler.limit",
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.append",
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.count",
	"net.sf.l2j.commons.logging.handler.ChatLogHandler.limit",
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.count",
	"net.sf.l2j.commons.logging.handler.ErrorLogHandler.limit",
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.append",
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.count",
	"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.limit",
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.append",
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.count",
	"net.sf.l2j.commons.logging.handler.ItemLogHandler.limit",
}

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

func TestConfigReportsUnimplementedRotationKeys(t *testing.T) {
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
	if !reflect.DeepEqual(cfg.UnsupportedKeys, unsupportedRotationKeys) {
		t.Fatalf("UnsupportedKeys = %#v, want %#v", cfg.UnsupportedKeys, unsupportedRotationKeys)
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
	if !reflect.DeepEqual(cfg.UnsupportedKeys, unsupportedRotationKeys) {
		t.Fatalf("UnsupportedKeys = %#v, want %#v", cfg.UnsupportedKeys, unsupportedRotationKeys)
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

package logging

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/rs/zerolog"
)

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
	runtime.Chat.Info().Str("from", "player").Msg("hello")
	runtime.GMAudit.Info().Str("gm", "admin").Msg("teleport")
	runtime.Item.Info().Int("item", 57).Msg("add")

	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}

	assertFileContains(t, runtime.Path(SinkConsole), "server started")
	assertFileContains(t, runtime.Path(SinkError), "boot failed")
	assertFileContains(t, runtime.Path(SinkChat), "hello")
	assertFileContains(t, runtime.Path(SinkGMAudit), "teleport")
	assertFileContains(t, runtime.Path(SinkItem), "add")
	if !strings.Contains(stderr.String(), "server started") {
		t.Fatalf("stderr = %q, want server log", stderr.String())
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

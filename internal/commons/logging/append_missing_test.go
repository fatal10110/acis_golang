package logging

import (
	"bufio"
	"bytes"
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/rs/zerolog"
)

const missingPropertyMsg = "config property missing; using default value"

// handlerProperties writes every handler key the shipped logging.properties
// carries except .append, plus appendLines verbatim.
func handlerProperties(appendLines string) string {
	var b strings.Builder
	b.WriteString(".level = INFO\njava.util.logging.ConsoleHandler.level = FINE\n")
	for _, prefix := range fileHandlerPropertyKey {
		b.WriteString(prefix + ".pattern = log/x%g.txt\n")
		b.WriteString(prefix + ".limit = 1000000\n")
		b.WriteString(prefix + ".count = 5\n")
		b.WriteString(prefix + ".level = INFO\n")
	}
	b.WriteString(appendLines)
	return b.String()
}

// missingKeys parses the config package's missing-property warnings out of a
// zerolog JSON stream (this package names the message field "msg") and
// returns the keys they name.
func missingKeys(t *testing.T, output *bytes.Buffer) []string {
	t.Helper()
	var keys []string
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		var entry struct {
			Message string `json:"msg"`
			Key     string `json:"key"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("decode warning %q: %v", scanner.Text(), err)
		}
		if entry.Message == missingPropertyMsg {
			keys = append(keys, entry.Key)
		}
	}
	return keys
}

func configFromText(t *testing.T, text string) (Config, []string) {
	t.Helper()
	props, err := config.ParseString(text)
	if err != nil {
		t.Fatalf("ParseString(): %v", err)
	}
	var output bytes.Buffer
	config.SetLogger(zerolog.New(&output))
	t.Cleanup(func() { config.SetLogger(zerolog.Nop()) })

	cfg, err := ConfigFromProperties(props)
	if err != nil {
		t.Fatalf("ConfigFromProperties(): %v", err)
	}
	return cfg, missingKeys(t, &output)
}

// The shipped logging.properties sets .append only for the chat, GM audit and
// item handlers; the console and error handlers rely on the file-handler
// default (false). An absent .append key must not log a missing-property
// warning, and the defaults must not change.
func TestConfigAppendKeysMissingDoNotWarn(t *testing.T) {
	t.Run("shipped layout", func(t *testing.T) {
		cfg, missing := configFromText(t, handlerProperties(
			"net.sf.l2j.commons.logging.handler.ChatLogHandler.append = true\n"+
				"net.sf.l2j.commons.logging.handler.GMAuditLogHandler.append = true\n"+
				"net.sf.l2j.commons.logging.handler.ItemLogHandler.append = true\n"))
		if len(missing) != 0 {
			t.Fatalf("missing-property warnings = %v, want none", missing)
		}
		want := map[Sink]bool{SinkConsole: false, SinkError: false, SinkChat: true, SinkGMAudit: true, SinkItem: true}
		for sink, append := range want {
			if cfg.Append[sink] != append {
				t.Errorf("Append[%v] = %v, want %v", sink, cfg.Append[sink], append)
			}
		}
	})

	t.Run("no append keys", func(t *testing.T) {
		cfg, missing := configFromText(t, handlerProperties(""))
		if len(missing) != 0 {
			t.Fatalf("missing-property warnings = %v, want none", missing)
		}
		if !maps.Equal(cfg.Append, DefaultConfig().Append) {
			t.Fatalf("Append = %v, want defaults %v", cfg.Append, DefaultConfig().Append)
		}
	})

	t.Run("explicit false", func(t *testing.T) {
		cfg, _ := configFromText(t, handlerProperties("net.sf.l2j.commons.logging.handler.ChatLogHandler.append = false\n"))
		if cfg.Append[SinkChat] {
			t.Fatal("Append[SinkChat] = true, want false")
		}
	})
}

// Only .append is silent: the other handler keys still warn when absent.
func TestConfigOtherHandlerKeysMissingStillWarn(t *testing.T) {
	text := handlerProperties("")
	const limitKey = "net.sf.l2j.commons.logging.handler.ErrorLogHandler.limit"
	const countKey = "java.util.logging.FileHandler.count"
	const patternKey = "net.sf.l2j.commons.logging.handler.ItemLogHandler.pattern"
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, limitKey) || strings.HasPrefix(line, countKey) || strings.HasPrefix(line, patternKey) {
			continue
		}
		kept = append(kept, line)
	}

	_, missing := configFromText(t, strings.Join(kept, "\n"))
	got := strings.Join(missing, ",")
	for _, key := range []string{limitKey, countKey, patternKey} {
		if !strings.Contains(got, key) {
			t.Errorf("missing-property warnings = %v, want one for %s", missing, key)
		}
	}
	if len(missing) != 3 {
		t.Errorf("missing-property warnings = %v, want exactly 3", missing)
	}
}

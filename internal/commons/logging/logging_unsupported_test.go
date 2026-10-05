package logging

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
)

const unsupportedWarning = "logging.properties keys not supported; ignored"

// setupWithProperties builds a runtime from text and returns its stderr output
// after the runtime is closed.
func setupWithProperties(t *testing.T, text string) string {
	t.Helper()
	props, err := config.ParseString(text)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ConfigFromProperties(props)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	rt, err := Setup(t.TempDir(), cfg, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	return stderr.String()
}

func TestSetupWarnsUnsupportedKeysOnce(t *testing.T) {
	out := setupWithProperties(t, `
.level = INFO
java.util.logging.FileHandler.encoding = UTF-8
handlerz = typo
`)

	var warnings []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("stderr line is not JSON: %v\n%s", err, line)
		}
		if event["msg"] == unsupportedWarning {
			warnings = append(warnings, event)
		}
	}
	if len(warnings) != 1 {
		t.Fatalf("unsupported-key warnings = %d, want 1\n%s", len(warnings), out)
	}
	if warnings[0]["level"] != "warn" {
		t.Fatalf("level = %v, want warn", warnings[0]["level"])
	}
	want := []any{"handlerz", "java.util.logging.FileHandler.encoding"}
	if !reflect.DeepEqual(warnings[0]["keys"], want) {
		t.Fatalf("keys = %#v, want %#v", warnings[0]["keys"], want)
	}
}

func TestSetupSilentWhenAllKeysSupported(t *testing.T) {
	out := setupWithProperties(t, `
.level = INFO
java.util.logging.ConsoleHandler.level = FINER
java.util.logging.FileHandler.limit = 1000000
`)
	if strings.Contains(out, unsupportedWarning) {
		t.Fatalf("stderr = %q, want no unsupported-key warning", out)
	}
}

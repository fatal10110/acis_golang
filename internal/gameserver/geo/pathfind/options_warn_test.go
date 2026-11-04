package pathfind

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/rs/zerolog"
)

func optionsWithWarnings(t *testing.T, text string) (Options, string) {
	t.Helper()
	props, err := config.ParseString(text)
	if err != nil {
		t.Fatalf("ParseString(): %v", err)
	}
	var output bytes.Buffer
	config.SetLogger(zerolog.New(&output))
	t.Cleanup(func() { config.SetLogger(zerolog.Nop()) })

	got, err := OptionsFromProperties(props)
	if err != nil {
		t.Fatalf("OptionsFromProperties(): %v", err)
	}
	return got, output.String()
}

// Bidirectional is not a geoengine.properties key: the shipped file never
// carries it, so reading it would warn on every boot. It stays at its default.
func TestOptionsFromPropertiesBidirectionalNotAFileKey(t *testing.T) {
	const shipped = "MoveWeight = 10\nMoveWeightDiag = 14\nObstacleWeight = 30\nHeuristicWeight = 12\nMaxIterations = 10000\n"

	t.Run("shipped keys", func(t *testing.T) {
		got, log := optionsWithWarnings(t, shipped)
		if !got.Bidirectional {
			t.Fatal("Bidirectional = false, want default true")
		}
		if strings.Contains(log, "config property missing") {
			t.Fatalf("missing-property warning logged: %s", log)
		}
	})

	t.Run("file value ignored", func(t *testing.T) {
		got, _ := optionsWithWarnings(t, shipped+"Bidirectional = false\n")
		if !got.Bidirectional {
			t.Fatal("Bidirectional read from the file, want default true")
		}
	})

	t.Run("other keys still warn", func(t *testing.T) {
		got, log := optionsWithWarnings(t, "")
		if got != DefaultOptions() {
			t.Fatalf("OptionsFromProperties() = %#v, want %#v", got, DefaultOptions())
		}
		if !strings.Contains(log, `"key":"MoveWeight"`) {
			t.Fatalf("missing MoveWeight did not warn: %s", log)
		}
		if strings.Contains(log, "Bidirectional") {
			t.Fatalf("Bidirectional warned: %s", log)
		}
	})
}

package pathfind

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/config"
)

func TestOptionsFromProperties(t *testing.T) {
	props, err := config.ParseString(`
MoveWeight = 11
MoveWeightDiag = 15
ObstacleWeight = 31
HeuristicWeight = 13
MaxIterations = 1234
`)
	if err != nil {
		t.Fatalf("ParseString(): %v", err)
	}

	got, err := OptionsFromProperties(props)
	if err != nil {
		t.Fatalf("OptionsFromProperties(): %v", err)
	}

	want := Options{
		MoveWeight:      11,
		MoveWeightDiag:  15,
		ObstacleWeight:  31,
		HeuristicWeight: 13,
		MaxIterations:   1234,
		Bidirectional:   true,
	}
	if got != want {
		t.Fatalf("OptionsFromProperties() = %#v, want %#v", got, want)
	}
}

func TestOptionsFromPropertiesDefaultsAndErrors(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		props, err := config.ParseString("")
		if err != nil {
			t.Fatalf("ParseString(): %v", err)
		}

		got, err := OptionsFromProperties(props)
		if err != nil {
			t.Fatalf("OptionsFromProperties(): %v", err)
		}
		if got != DefaultOptions() {
			t.Fatalf("OptionsFromProperties() = %#v, want %#v", got, DefaultOptions())
		}
	})

	t.Run("invalid integer", func(t *testing.T) {
		props, err := config.ParseString("MoveWeight = nope\n")
		if err != nil {
			t.Fatalf("ParseString(): %v", err)
		}

		if _, err := OptionsFromProperties(props); err == nil {
			t.Fatal("OptionsFromProperties() error = nil, want error")
		}
	})
}

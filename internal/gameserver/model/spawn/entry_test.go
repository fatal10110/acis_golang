package spawn

import "testing"

func TestParsePositionsAllowsTrailingWeightedSeparator(t *testing.T) {
	positions, err := ParsePositions("1;2;3;4;60%;5;6;7;8;40%;")
	if err != nil {
		t.Fatalf("ParsePositions error: %v", err)
	}
	if got, want := len(positions), 2; got != want {
		t.Fatalf("len(positions) = %d, want %d", got, want)
	}
	if got, want := positions[1].Chance, 40; got != want {
		t.Fatalf("positions[1].Chance = %d, want %d", got, want)
	}
}

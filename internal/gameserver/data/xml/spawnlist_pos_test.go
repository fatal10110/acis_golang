package xml

import "testing"

func TestSpawnParsePositionsAllowsTrailingWeightedSeparator(t *testing.T) {
	positions, err := parsePositions("1;2;3;4;60%;5;6;7;8;40%;")
	if err != nil {
		t.Fatalf("parsePositions error: %v", err)
	}
	if got, want := len(positions), 2; got != want {
		t.Fatalf("len(positions) = %d, want %d", got, want)
	}
	if got, want := positions[1].Chance, 40; got != want {
		t.Fatalf("positions[1].Chance = %d, want %d", got, want)
	}
	if got := positions[0]; got.Location.X != 1 || got.Location.Y != 2 || got.Location.Z != 3 || got.Heading != 4 {
		t.Fatalf("positions[0] = %+v, want x/y/z/heading 1/2/3/4", got)
	}
}

func TestSpawnParsePositionsRejectsMalformedCoordinate(t *testing.T) {
	if _, err := parsePositions("1;x;3;4;60%"); err == nil {
		t.Fatal("parsePositions error = nil, want a parse failure for a non-numeric y")
	}
}

// Java (SpawnManager.java:205-213) sizes the weighted array at
// loc.length/5 and loops only over complete groups: a trailing partial
// group is silently dropped without being parsed, even if malformed.
func TestSpawnParsePositionsDropsIncompleteTrailingWeightedGroup(t *testing.T) {
	positions, err := parsePositions("1;2;3;4;60%;garbage")
	if err != nil {
		t.Fatalf("parsePositions error: %v", err)
	}
	if got, want := len(positions), 1; got != want {
		t.Fatalf("len(positions) = %d, want %d", got, want)
	}
	if got := positions[0]; got.Location.X != 1 || got.Location.Y != 2 || got.Location.Z != 3 || got.Heading != 4 || got.Chance != 60 {
		t.Fatalf("positions[0] = %+v, want x/y/z/heading/chance 1/2/3/4/60", got)
	}
}

func TestSpawnParsePositionsFloorsGroupCountForNonMultipleTokenCounts(t *testing.T) {
	// 9 tokens: one complete weighted group (5) plus a dropped partial (4).
	positions, err := parsePositions("1;2;3;4;10%;5;6;7;8")
	if err != nil {
		t.Fatalf("parsePositions error: %v", err)
	}
	if got, want := len(positions), 1; got != want {
		t.Fatalf("len(positions) = %d, want %d", got, want)
	}
}

func TestSpawnParsePositionsRejectsFewerThanFourTokens(t *testing.T) {
	if _, err := parsePositions("1;2;3"); err == nil {
		t.Fatal("parsePositions error = nil, want a failure for an incomplete fixed tuple")
	}
}

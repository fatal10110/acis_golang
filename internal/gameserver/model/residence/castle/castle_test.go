package castle

import "testing"

func TestParseSpawnLocation(t *testing.T) {
	pos, heading, err := parseSpawnLocation("10;-20;30;16384")
	if err != nil {
		t.Fatalf("parseSpawnLocation error: %v", err)
	}
	if pos.X != 10 || pos.Y != -20 || pos.Z != 30 || heading != 16384 {
		t.Fatalf("parseSpawnLocation() = %+v, %d; want {10 -20 30}, 16384", pos, heading)
	}
}

func TestParseSpawnLocationRejectsMalformed(t *testing.T) {
	for _, raw := range []string{"10;20;30", "10;20;30;40;50", "10;20;z;40"} {
		if _, _, err := parseSpawnLocation(raw); err == nil {
			t.Fatalf("parseSpawnLocation(%q) error = nil, want a parse failure", raw)
		}
	}
}

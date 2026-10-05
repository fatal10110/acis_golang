package spawn

import "testing"

// TestMakerResidenceParam pins NpcMaker's spawnTime parse as
// MultiSpawn.doSpawn reads it: split on the brackets into exactly a kind
// and its parameters, a known kind other than door_open, and a first
// parameter of ASCII digits (StringUtil.isDigit) naming the residence.
func TestMakerResidenceParam(t *testing.T) {
	for _, tt := range []struct {
		spawnTime string
		want      int
		ok        bool
	}{
		{"siege_warfare_start(7)", 7, true},
		{"pc_siege_warfare_start(9)", 9, true},
		{"agit_defend_warfare_start(64)", 64, true},
		{"agit_final_start(63;champion;5)", 63, true},
		{"SIEGE_WARFARE_START(3)", 3, true}, // the kind matches ignoring case
		{"door_open([kuruma_parent])", 0, false},
		{"door_open(5)", 0, false},
		{"unknown_start(5)", 0, false},
		{"siege_warfare_start([gludio])", 0, false},
		{"siege_warfare_start(;5)", 0, false},
		{"siege_warfare_start(-5)", 0, false},
		{"siege_warfare_start(5)(6)", 0, false},
		{"siege_warfare_start()", 0, false},
		{"(5)", 0, false},
		{"siege_warfare_start(99999999999)", 0, false},
		{"", 0, false},
	} {
		got, ok := (&Maker{SpawnTime: tt.spawnTime}).ResidenceParam()
		if got != tt.want || ok != tt.ok {
			t.Errorf("ResidenceParam(%q) = (%d, %v), want (%d, %v)", tt.spawnTime, got, ok, tt.want, tt.ok)
		}
	}
	if _, ok := (*Maker)(nil).ResidenceParam(); ok {
		t.Error("a nil maker names a residence")
	}
}

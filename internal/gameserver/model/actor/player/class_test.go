package player

import "testing"

func TestClassLevel(t *testing.T) {
	tests := []struct {
		id        int
		wantLevel int
		wantOK    bool
	}{
		{id: 0, wantLevel: 0, wantOK: true},   // base
		{id: 1, wantLevel: 1, wantOK: true},   // first occupation change
		{id: 2, wantLevel: 2, wantOK: true},   // second occupation change
		{id: 88, wantLevel: 3, wantOK: true},  // third class
		{id: 118, wantLevel: 3, wantOK: true}, // third class, last id
		{id: 999, wantLevel: 0, wantOK: false},
		{id: 58, wantLevel: 0, wantOK: false}, // reserved gap
	}
	for _, tt := range tests {
		level, ok := ClassLevel(tt.id)
		if level != tt.wantLevel || ok != tt.wantOK {
			t.Errorf("ClassLevel(%d) = (%d, %v), want (%d, %v)", tt.id, level, ok, tt.wantLevel, tt.wantOK)
		}
	}
}

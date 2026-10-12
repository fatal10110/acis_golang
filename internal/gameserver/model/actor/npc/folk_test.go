package npc

import "testing"

// TestFolkKindLeavesOutClickInertCivilians pins which civilian types spawn
// as a talkable Folk. ChristmasTree answers every click with ActionFailed
// alone (no selection, no talk), so it is no Folk; a plain service type
// such as Merchant is, and so is the castle holy artifact (HolyThing).
func TestFolkKindLeavesOutClickInertCivilians(t *testing.T) {
	for _, tc := range []struct {
		kind string
		folk bool
	}{
		{"ChristmasTree", false},
		{"HolyThing", true},
		{"Merchant", true},
	} {
		inst, err := NewInstance(1, &Template{ID: 13006, TemplateID: 13006, Type: tc.kind, Level: 1, HPMax: 100})
		if err != nil {
			t.Fatalf("%s: new instance: %v", tc.kind, err)
		}
		if got := FolkKind(inst); got != tc.folk {
			t.Fatalf("FolkKind(%s) = %v, want %v", tc.kind, got, tc.folk)
		}
		if _, err := NewFolk(inst); (err == nil) != tc.folk {
			t.Fatalf("NewFolk(%s) error = %v, want folk=%v", tc.kind, err, tc.folk)
		}
		if Attackable(inst) {
			t.Fatalf("Attackable(%s) = true, want a civilian type the spawner skips unless it is a Folk", tc.kind)
		}
	}
}

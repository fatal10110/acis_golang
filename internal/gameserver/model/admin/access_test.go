package admin

import "testing"

// TestHasAccess pins the command-rights rule against an access table with
// a child chain: a level runs a command at its own level or at one its
// child levels reach, never one above it; an undefined command or required
// level refuses everyone; and a cycle of child levels ends refused.
func TestHasAccess(t *testing.T) {
	levels := []AccessLevel{
		{Level: 0, Name: "User"},
		{Level: 2, Name: "Test GM", ChildLevel: 1},
		{Level: 1, Name: "Chat Moderator"},
		{Level: 7, Name: "Admin", ChildLevel: 2},
		{Level: 8, Name: "Master", ChildLevel: 7},
		{Level: 20, Name: "Loop A", ChildLevel: 21},
		{Level: 21, Name: "Loop B", ChildLevel: 20},
	}
	commands := []Command{
		{Name: "admin_admin", AccessLevel: 7},
		{Name: "admin_chat", AccessLevel: 1},
		{Name: "admin_orphan", AccessLevel: 5},
	}
	data, err := NewData(levels, commands)
	if err != nil {
		t.Fatalf("NewData() error: %v", err)
	}
	for _, tc := range []struct {
		command string
		level   int
		want    bool
	}{
		{"admin_admin", 7, true},
		{"admin_admin", 8, true},
		{"admin_admin", 2, false},
		{"admin_admin", 0, false},
		{"ADMIN_ADMIN", 7, true},
		{"admin_chat", 8, true},
		{"admin_chat", 2, true},
		{"admin_chat", 0, false},
		{"admin_orphan", 8, false},
		{"admin_unlisted", 8, false},
		{"admin_admin", 20, false},
	} {
		access, _ := data.AccessLevel(tc.level)
		if got := data.HasAccess(tc.command, access); got != tc.want {
			t.Errorf("HasAccess(%q, level %d) = %v, want %v", tc.command, tc.level, got, tc.want)
		}
	}
	if !data.Defines("admin_admin") || data.Defines("ADMIN_ADMIN") || data.Defines("admin_unlisted") {
		t.Error("Defines must match the table's spelling exactly")
	}
	var none *Data
	if none.HasAccess("admin_admin", AccessLevel{Level: 8}) || none.Defines("admin_admin") {
		t.Error("a nil table must grant and define nothing")
	}
}
